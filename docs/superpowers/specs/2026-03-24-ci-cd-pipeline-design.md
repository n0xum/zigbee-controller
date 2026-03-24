# CI/CD Pipeline Design

**Date:** 2026-03-24
**Project:** zigbee-controller
**Repo:** github.com/n0xum/zigbee-controller (Go module path: `github.com/ak/zigbee-controller` — the two differ intentionally; the GitHub owner is `n0xum`, so `ghcr.io/n0xum/zigbee-controller` is correct and `GITHUB_TOKEN` will have push permission automatically)

---

## Goal

Add a testing and deployment pipeline so that every push to `main` is automatically tested, containerised, and deployed to the home server (UGreen DXP2800).

---

## Architecture

Single GitHub Actions workflow file with two jobs connected by a dependency:

```
push / PR
    └── test (go vet + go test ./...)
              │ (main only)
              └── build-push-deploy
                    ├── build Docker image
                    ├── push to ghcr.io
                    └── SSH → docker compose pull && up -d bridge
```

- `test` runs on every push and every pull request.
- `build-push-deploy` runs only on push to `main` and only if `test` passes.

---

## Components

### 1. Dockerfile

Multi-stage build at the repo root.

| Stage | Base image | Purpose |
|---|---|---|
| `builder` | `golang:1.24-alpine` | Compile static binary |
| `runtime` | `alpine:3.21` | Run binary; no Go toolchain |

Build environment for the binary: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.

`CGO_ENABLED=0` is safe here because all DNS/mDNS work is handled by `miekg/dns` (a pure-Go DNS library pulled in transitively via `brutella/dnssd`), so no C resolver is needed at runtime.

`GOARCH=amd64` is specified explicitly so the binary targets the DXP2800 (x86_64) regardless of the Actions runner architecture.

Runtime stage details:
- Non-root user `appuser` created with `adduser -D appuser`.
- `hap-data` directory created and owned by `appuser` before switching to that user: `RUN mkdir -p /app/hap-data && chown appuser /app/hap-data`.
- Binary is the `ENTRYPOINT` (not `CMD`), so `CMD` is available for passing a custom config path: `ENTRYPOINT ["/app/bridge"]`.

Final image size: ~15 MB.

### 2. docker-compose.yml changes

Two changes to the existing `docker-compose.yml`:

**Add `restart: unless-stopped` to `mosquitto`** (currently missing; all services should survive reboots).

**Add `bridge` service:**

```yaml
bridge:
  image: ghcr.io/n0xum/zigbee-controller:latest
  restart: unless-stopped
  depends_on: [mosquitto]
  network_mode: host
  volumes:
    - ./config.yaml:/app/config.yaml:ro
    - ./hap-data:/app/hap-data
```

`network_mode: host` is required for HAP/mDNS — HomeKit discovery uses Bonjour/mDNS which does not cross Docker bridge networks cleanly.

**Important:** because `bridge` uses `network_mode: host`, it cannot use Docker's internal DNS to reach other services by name. The `config.yaml` on the server must set the MQTT broker address to `localhost:1883` (not `mosquitto:1883`).

`hap-data` is a bind mount relative to the compose file; Docker creates it automatically on first run. The container writes pairing state here, so it must survive container restarts.

### 3. GitHub Actions workflow

**File:** `.github/workflows/ci-cd.yml`

**Job: `test`**
- Trigger: all pushes and pull requests
- Steps: checkout → setup-go (1.24, module cache enabled) → `go vet ./...` → `go test ./...`

**Job: `build-push-deploy`**
- Trigger: push to `main` only; `needs: test`
- Steps:
  1. Checkout
  2. Log in to `ghcr.io` using `GITHUB_TOKEN` (no extra secret needed)
  3. Build and push image with `docker/build-push-action`:
     - Tags: `ghcr.io/n0xum/zigbee-controller:latest` and `ghcr.io/n0xum/zigbee-controller:${{ github.sha }}`
     - The SHA tag enables manual rollback: `docker compose up -d` with the SHA tag on the server
  4. SSH into server using `appleboy/ssh-action@v1`:
     - `DEPLOY_PATH` is passed via the action's `envs:` parameter so it is available as `$DEPLOY_PATH` in the script body
     - Script: `cd $DEPLOY_PATH && docker compose pull bridge && docker compose up -d bridge`

**Required secrets** (set in GitHub repo → Settings → Secrets):

| Secret | Value |
|---|---|
| `DEPLOY_HOST` | IP or hostname of DXP2800 |
| `DEPLOY_USER` | SSH username |
| `DEPLOY_SSH_KEY` | Private SSH key — Ed25519 or RSA, in either PEM (`-----BEGIN RSA PRIVATE KEY-----`) or OpenSSH format (`-----BEGIN OPENSSH PRIVATE KEY-----`); add the corresponding public key to `~/.ssh/authorized_keys` on the server |
| `DEPLOY_PATH` | Absolute path to `docker-compose.yml` on server (e.g. `/opt/zigbee-controller`) |

### 4. Makefile additions

```makefile
test:
	go test ./...

lint:
	go vet ./...
```

Consistent with CI so developers can run the same checks locally.

---

## Data flow on deploy

1. Developer merges PR into `main`.
2. GitHub Actions runs `test` job (`go vet` then `go test ./...`). Either failure stops the pipeline.
3. If tests pass, `build-push-deploy` builds the Docker image and pushes two tags (`latest` and the commit SHA) to `ghcr.io/n0xum/zigbee-controller`.
4. SSH action connects to DXP2800 and runs `docker compose pull bridge && docker compose up -d bridge` in `$DEPLOY_PATH`.
5. Docker pulls the new image and restarts only the `bridge` container; `mosquitto` and `zigbee2mqtt` are unaffected.

---

## Error handling

- `go vet ./...` failure stops the pipeline — no image is built or deployed.
- `go test ./...` failure stops the pipeline — no image is built or deployed.
- Docker build failure skips the SSH deploy step.
- SSH step failure marks the workflow failed and triggers GitHub's standard notification.
- `restart: unless-stopped` ensures the container recovers from crashes without pipeline involvement.
- If the new container starts but crashes (e.g., bad config on server), the old container is gone and the new one enters a restart loop. There is no automatic rollback — manual intervention is required (roll back with `docker compose up -d` using the previous SHA tag).

---

## What is NOT in scope

- Multi-architecture builds (amd64 only; DXP2800 is x86_64).
- Staging environment.
- Automatic rollback on container crash.
- Secret rotation automation.
