# CI/CD Pipeline Design

**Date:** 2026-03-24
**Project:** zigbee-controller
**Repo:** github.com/n0xum/zigbee-controller

---

## Goal

Add a testing and deployment pipeline so that every push to `main` is automatically tested, containerised, and deployed to the home server (UGreen DXP2800).

---

## Architecture

Single GitHub Actions workflow file with two jobs connected by a dependency:

```
push / PR
    └── test (go test ./...)
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
| `builder` | `golang:1.24-alpine` | Compile static binary (`CGO_ENABLED=0 GOOS=linux`) |
| `runtime` | `alpine:3.21` | Run binary; no Go toolchain |

- Binary copied from builder to `/app/bridge` in runtime stage.
- Non-root user (`appuser`) created and used.
- Final image size: ~15 MB.

### 2. docker-compose.yml (bridge service)

New `bridge` service added to the existing `docker-compose.yml`:

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

`network_mode: host` is required for HAP/mDNS (HomeKit discovery does not work across Docker bridge networks).
`hap-data` volume preserves HomeKit pairing state across container restarts.

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
  3. Build and push image with `docker/build-push-action`, tag `latest`
  4. SSH into server, `cd $DEPLOY_PATH && docker compose pull bridge && docker compose up -d bridge`

**Required secrets** (set in GitHub repo → Settings → Secrets):

| Secret | Value |
|---|---|
| `DEPLOY_HOST` | IP or hostname of DXP2800 |
| `DEPLOY_USER` | SSH username |
| `DEPLOY_SSH_KEY` | Private SSH key (PEM); add public key to `~/.ssh/authorized_keys` on server |
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
2. GitHub Actions runs `test` job (vet + unit tests).
3. If tests pass, `build-push-deploy` builds the Docker image and pushes to `ghcr.io/n0xum/zigbee-controller:latest`.
4. SSH action connects to DXP2800, runs `docker compose pull bridge && docker compose up -d bridge` in `$DEPLOY_PATH`.
5. Docker pulls the new image and restarts only the `bridge` container; `mosquitto` and `zigbee2mqtt` are unaffected.

---

## Error handling

- If `go test ./...` fails, the workflow stops — no image is built or deployed.
- If the Docker build fails, the SSH deploy step is skipped.
- If the SSH step fails, GitHub Actions marks the workflow as failed and notifies via the standard GitHub notification channel.
- The `unless-stopped` restart policy ensures the container recovers from crashes without pipeline involvement.

---

## What is NOT in scope

- Multi-architecture builds (amd64 only; DXP2800 is x86_64).
- Staging environment.
- Rollback automation (manual `docker compose up -d` with a previous image tag if needed).
- Secret rotation automation.
