# CI/CD Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Dockerfile, update docker-compose.yml, create a GitHub Actions workflow, and extend the Makefile so every push to `main` is tested, containerised, pushed to ghcr.io, and deployed to the home server via SSH.

**Architecture:** A single GitHub Actions workflow runs `go vet` + `go test` on every push and PR; on `main` it also builds a Docker image (multi-stage, static binary, non-root user), pushes two tags (`latest` + commit SHA) to `ghcr.io/n0xum/zigbee-controller`, then SSHes into the DXP2800 and runs `docker compose pull bridge && docker compose up -d bridge`.

**Tech Stack:** Go 1.24, Docker (multi-stage Alpine), GitHub Actions, ghcr.io, `appleboy/ssh-action@v1`, `docker/build-push-action@v5`

---

## File Map

| File | Change |
|---|---|
| `Dockerfile` | Create — multi-stage build |
| `docker-compose.yml` | Modify — add `bridge` service, add `restart` to `mosquitto` |
| `.github/workflows/ci-cd.yml` | Create — test + build/push/deploy jobs |
| `Makefile` | Modify — add `test` and `lint` targets |

**Note:** This plan is executed on the `feature/ci-cd-pipeline` branch (worktree at `.worktrees/ci-cd`). It must be executed after the `feature/zigbee-controller` branch has been merged into `main`, since the Dockerfile needs `go.mod`, `go.sum`, and `cmd/bridge/` to be present.

---

## Task 1: Dockerfile

**Files:**
- Create: `Dockerfile`

There are no unit tests for a Dockerfile. The verification step is a successful `docker build`.

- [ ] **Step 1: Create Dockerfile**

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bridge ./cmd/bridge

FROM alpine:3.21

RUN adduser -D appuser && \
    mkdir -p /app/hap-data && \
    chown appuser /app/hap-data

WORKDIR /app
COPY --from=builder /build/bridge /app/bridge

USER appuser

ENTRYPOINT ["/app/bridge"]
```

Key decisions:
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` — static binary for x86_64 (DXP2800). CGO disabled because all DNS/mDNS is handled by `miekg/dns` (pure Go).
- `GOARCH=amd64` explicit — prevents wrong-arch binary if runner is ever non-amd64.
- `ENTRYPOINT` (not `CMD`) — so `CMD` is free for passing a custom config path via `os.Args[1]`.
- `hap-data` directory created and owned by `appuser` before `USER appuser` — required so the HAP pairing state file can be written.

- [ ] **Step 2: Verify the build succeeds**

Run from the repo root:
```bash
docker build -t zigbee-controller:local .
```
Expected: build completes, no errors, image created.

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "build: multi-stage Dockerfile for bridge binary"
```

---

## Task 2: docker-compose.yml update

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Replace docker-compose.yml with updated version**

```yaml
services:
  mosquitto:
    image: eclipse-mosquitto:2
    ports:
      - "1883:1883"
    volumes:
      - ./mosquitto.conf:/mosquitto/config/mosquitto.conf
    restart: unless-stopped

  zigbee2mqtt:
    image: koenkk/zigbee2mqtt:latest
    depends_on:
      - mosquitto
    volumes:
      - ./zigbee2mqtt:/app/data
      - /run/udev:/run/udev:ro
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
    environment:
      - TZ=Europe/Berlin
    restart: unless-stopped

  bridge:
    image: ghcr.io/n0xum/zigbee-controller:latest
    restart: unless-stopped
    depends_on:
      - mosquitto
    network_mode: host
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./hap-data:/app/hap-data
```

Key decisions:
- `restart: unless-stopped` added to `mosquitto` — was missing, needed for server reboots.
- `bridge` uses `network_mode: host` — required for HAP/mDNS (HomeKit discovery uses Bonjour which does not cross Docker bridge networks).
- **Important:** because `bridge` uses `network_mode: host`, the MQTT broker address in `config.yaml` on the server must be `localhost:1883` (not `mosquitto:1883` — Docker service DNS does not work in host network mode).
- `hap-data` bind mount preserves HomeKit pairing state across container restarts.
- `config.yaml` mounted read-only.

- [ ] **Step 2: Validate the compose file**

```bash
docker compose config
```
Expected: YAML printed without errors.

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "build: add bridge service to docker-compose, fix mosquitto restart policy"
```

---

## Task 3: GitHub Actions workflow

**Files:**
- Create: `.github/workflows/ci-cd.yml`

- [ ] **Step 1: Create workflow directory**

```bash
mkdir -p .github/workflows
```

- [ ] **Step 2: Create .github/workflows/ci-cd.yml**

```yaml
name: CI/CD

on:
  push:
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./...

  build-push-deploy:
    needs: test
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: |
            ghcr.io/n0xum/zigbee-controller:latest
            ghcr.io/n0xum/zigbee-controller:${{ github.sha }}

      - uses: appleboy/ssh-action@v1
        with:
          host: ${{ secrets.DEPLOY_HOST }}
          username: ${{ secrets.DEPLOY_USER }}
          key: ${{ secrets.DEPLOY_SSH_KEY }}
          envs: DEPLOY_PATH
          script: cd $DEPLOY_PATH && docker compose pull bridge && docker compose up -d bridge
        env:
          DEPLOY_PATH: ${{ secrets.DEPLOY_PATH }}
```

Key decisions:
- `on: push` (no branch filter) — `test` job runs on all branches and PRs.
- `build-push-deploy` is gated by `if: github.ref == 'refs/heads/main' && github.event_name == 'push'` — only deploys from main.
- `permissions: packages: write` — required to push to ghcr.io using `GITHUB_TOKEN`.
- Two image tags: `latest` and `${{ github.sha }}` — the SHA tag enables manual rollback.
- `appleboy/ssh-action@v1` with `envs: DEPLOY_PATH` and `env: DEPLOY_PATH: ${{ secrets.DEPLOY_PATH }}` — this is how the secret is passed into the remote script body as `$DEPLOY_PATH`.
- Only `bridge` is restarted — `mosquitto` and `zigbee2mqtt` are unaffected by a code deploy.

- [ ] **Step 3: Validate workflow YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci-cd.yml'))" && echo "YAML valid"
```
Expected: `YAML valid`

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci-cd.yml
git commit -m "ci: GitHub Actions workflow for test, build, push, and deploy"
```

---

## Task 4: Makefile additions

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update Makefile**

Replace the first line and add two new targets:

```makefile
.PHONY: build run docker-up docker-logs tidy test lint

build:
	go build -o bin/bridge ./cmd/bridge

run:
	go run ./cmd/bridge

docker-up:
	docker compose up -d

docker-logs:
	docker compose logs -f zigbee2mqtt

tidy:
	go mod tidy

test:
	go test ./...

lint:
	go vet ./...
```

- [ ] **Step 2: Verify targets work**

```bash
make lint && make test
```
Expected: `go vet ./...` exits 0, all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add test and lint Makefile targets matching CI"
```

---

## Setup reminder: GitHub secrets

Before the first deployment, add these four secrets in GitHub → repo → Settings → Secrets and variables → Actions:

| Secret | Example value |
|---|---|
| `DEPLOY_HOST` | `192.168.1.100` or `dxp2800.local` |
| `DEPLOY_USER` | `ak` |
| `DEPLOY_SSH_KEY` | Contents of `~/.ssh/id_ed25519` (Ed25519 or RSA, OpenSSH or PEM format) |
| `DEPLOY_PATH` | `/opt/zigbee-controller` |

The corresponding public key must be in `~/.ssh/authorized_keys` on the DXP2800.
