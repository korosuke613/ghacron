# ghacron

GitHub Actions `schedule` events (cron triggers) have a known issue where delays of over an hour can occur. ghacron is a Go service that reads annotations from workflow files and fires `workflow_dispatch` events on time.

## How It Works

- Add annotations like `# ghacron: "0 8 * * *"` to your workflow files
- The service scans repositories every 5 minutes and detects annotations
- Fires `workflow_dispatch` according to the cron expression
- State is persisted via GitHub Actions Variables (no PVC required)

## Annotation Format

```yaml
on:
  # ghacron: "0 8 * * *"
  workflow_dispatch:
```

- `workflow_dispatch:` must be included under `on:`
- Multiple annotations per file are supported
- Cron expressions use the standard 5-field format (minute hour day month weekday)

## Requirements

- Go 1.25 or later
- GitHub App (App ID + Private Key)
  - Required permissions: `contents: read`, `actions: write`, `variables: write`, `metadata: read`

## Usage

```bash
./ghacron [options]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `config/config.yaml` | Path to config file |
| `-version` | — | Show version and exit |

The default config path can also be overridden via the `GHACRON_CONFIG` environment variable.

```bash
# Basic
GH_APP_ID=123456 GH_APP_PRIVATE_KEY="$(cat key.pem)" ./ghacron

# Custom config path
./ghacron -config /etc/ghacron/config.yaml
```

## Configuration

Config file format:

```yaml
github:
  app_id: ${GH_APP_ID}
  private_key: "${GH_APP_PRIVATE_KEY}"

reconcile:
  interval_minutes: 5
  duplicate_guard_seconds: 60
  dry_run: false
  timezone: "Asia/Tokyo"    # IANA timezone for cron schedule evaluation

log:
  level: "info"
  format: "json"            # "json" or "text"

webapi:
  enabled: true
  host: "0.0.0.0"
  port: 8080
```

## API Endpoints

The web API server is enabled by default on port 8080. All responses are JSON.

### `GET /healthz`

Health check for liveness probes.

```json
{"status": "ok"}
```

### `GET /status`

Service status including uptime and reconciliation state.

```json
{
  "uptime_seconds": 3600.5,
  "registered_jobs": 3,
  "last_reconcile": "2026-02-24T09:00:00Z"
}
```

### `GET /jobs`

List of registered cron jobs with next scheduled run time.

```json
[
  {
    "owner": "myorg",
    "repo": "myrepo",
    "workflow_file": "ci.yml",
    "cron_expr": "0 8 * * *",
    "next_run": "2026-02-25T08:00:00Z"
  }
]
```

### `GET /config`

Public configuration (credentials are not exposed).

```json
{
  "github": {"app_id": 123456},
  "reconcile": {"interval_minutes": 5, "duplicate_guard_seconds": 60, "dry_run": false},
  "log": {"level": "info", "format": "json"},
  "webapi": {"enabled": true, "host": "0.0.0.0", "port": 8080}
}
```

## Docker

```bash
# Build
docker build -t ghacron .

# Run
docker run -e GH_APP_ID=123456 -e GH_APP_PRIVATE_KEY="$(cat key.pem)" ghacron
```

## Kubernetes Deployment

```yaml
containers:
- name: ghacron
  image: ghcr.io/korosuke613/ghacron:latest
  env:
  - name: GH_APP_ID
    value: "123456"
  - name: GH_APP_PRIVATE_KEY
    valueFrom:
      secretKeyRef:
        name: ghacron-secrets
        key: private-key
```

## Development

```bash
# Build
go build -ldflags="-s -w -X main.version=$(git describe --tags --always)" -o ghacron main.go

# Run (dry-run)
GH_APP_ID=123456 GH_APP_PRIVATE_KEY="$(cat key.pem)" ./ghacron

# Test
go test ./...
```

### Release Strategy

Releases are triggered exclusively by GitHub Release events — pushing to `main` does not publish any artifacts.

1. Create a GitHub Release (manually or via `/generate-release` skill in Claude Code)
2. The `release.yml` workflow automatically:
   - Builds Go binaries for linux/amd64 and linux/arm64 via [GoReleaser](https://goreleaser.com/)
   - Builds and pushes multi-arch Docker images to `ghcr.io/korosuke613/ghacron`

**Tagging rules:**

| Release type | Example tag | Docker tags | `latest` |
|---|---|---|---|
| Stable | `v1.0.0` | `1.0.0`, `1.0`, `latest` | Yes |
| Release candidate | `v1.0.0-rc.1` | `1.0.0-rc.1` | No |

## Architecture

```
ghacron/
├── main.go              # Entry point
├── config/              # Configuration management
├── github/              # GitHub App authentication & API client
├── scanner/             # Workflow scanning & annotation parsing
├── scheduler/           # Cron job management & reconciliation
├── api/                 # Health/status API
├── Dockerfile
└── README.md
```

## References

- [cronium](https://zenn.dev/cybozu_ept/articles/run-github-actions-scheduled-workflows-on-time) — A prior implementation with a similar approach

## License

MIT License
