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
- Per-workflow timezone override via `CRON_TZ=` or `TZ=` prefix:

```yaml
on:
  # ghacron: "CRON_TZ=Asia/Tokyo 0 8 * * *"
  workflow_dispatch:
```

When specified, the prefix overrides the global `GHACRON_TIMEZONE` setting for that job. The value must be a valid [IANA timezone name](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones).

## Requirements

- Go 1.25 or later
- GitHub App (App ID + Private Key)
  - Required permissions: `contents: read`, `actions: write`, `variables: write`, `metadata: read`

## Usage

```bash
./ghacron [options]
```

| Flag | Description |
|------|-------------|
| `-version` | Show version and exit |

```bash
# Binary
GHACRON_APP_ID=123456 GHACRON_APP_PRIVATE_KEY="$(cat key.pem)" ./ghacron

# Docker
docker run \
  -e GHACRON_APP_ID=123456 \
  -e GHACRON_APP_PRIVATE_KEY="$(cat key.pem)" \
  ghcr.io/korosuke613/ghacron
```

## Configuration

All configuration is done via `GHACRON_*` environment variables.

| Environment Variable | Type | Default | Required | Description |
|---|---|---|---|---|
| `GHACRON_APP_ID` | int64 | — | Yes | GitHub App ID |
| `GHACRON_APP_PRIVATE_KEY` | string | — | Yes* | GitHub App Private Key (PEM) |
| `GHACRON_APP_PRIVATE_KEY_PATH` | string | — | Yes* | Private Key file path |
| `GHACRON_RECONCILE_INTERVAL_MINUTES` | int | `5` | No | Reconcile loop interval in minutes |
| `GHACRON_RECONCILE_DUPLICATE_GUARD_SECONDS` | int | `60` | No | Duplicate dispatch guard in seconds |
| `GHACRON_DRY_RUN` | bool | `false` | No | Dry-run mode |
| `GHACRON_TIMEZONE` | string | `UTC` | No | IANA timezone for cron schedule evaluation |
| `GHACRON_LOG_LEVEL` | string | `info` | No | Log level (debug/info/warn/error) |
| `GHACRON_LOG_FORMAT` | string | `json` | No | Log format (json/text) |
| `GHACRON_WEBAPI_ENABLED` | bool | `true` | No | Enable/disable web API server |
| `GHACRON_WEBAPI_HOST` | string | `0.0.0.0` | No | Web API listen host |
| `GHACRON_WEBAPI_PORT` | int | `8080` | No | Web API listen port |

*Either `GHACRON_APP_PRIVATE_KEY` or `GHACRON_APP_PRIVATE_KEY_PATH` is required. When both are set, `GHACRON_APP_PRIVATE_KEY` takes priority.

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

Registered cron jobs and annotations that failed validation.

```json
{
  "registered": [
    {
      "owner": "myorg",
      "repo": "myrepo",
      "workflow_file": "ci.yml",
      "cron_expr": "0 8 * * *",
      "next_run": "2026-02-25T08:00:00Z"
    }
  ],
  "skipped": [
    {
      "owner": "myorg",
      "repo": "myrepo",
      "workflow_file": "deploy.yml",
      "cron_expr": "CRON_TZ=Asis/Tokyo 0 8 * * *",
      "reason": "provided bad location Asis/Tokyo: unknown time zone Asis/Tokyo"
    }
  ]
}
```

### `GET /config`

Public configuration (credentials are not exposed).

```json
{
  "app_id": 123456,
  "reconcile_interval_minutes": 5,
  "reconcile_duplicate_guard_seconds": 60,
  "dry_run": false,
  "timezone": "UTC",
  "log_level": "info",
  "log_format": "json",
  "webapi_enabled": true,
  "webapi_host": "0.0.0.0",
  "webapi_port": 8080
}
```

## Docker

```bash
# Build
docker build -t ghacron .

# Run
docker run \
  -e GHACRON_APP_ID=123456 \
  -e GHACRON_APP_PRIVATE_KEY="$(cat key.pem)" \
  ghacron

# Run with all options
docker run \
  -e GHACRON_APP_ID=123456 \
  -e GHACRON_APP_PRIVATE_KEY="$(cat key.pem)" \
  -e GHACRON_TIMEZONE=Asia/Tokyo \
  -e GHACRON_DRY_RUN=true \
  ghacron
```

## Kubernetes Deployment

```yaml
containers:
- name: ghacron
  image: ghcr.io/korosuke613/ghacron:latest
  env:
  - name: GHACRON_APP_ID
    value: "123456"
  - name: GHACRON_APP_PRIVATE_KEY
    valueFrom:
      secretKeyRef:
        name: ghacron-secrets
        key: private-key
  - name: GHACRON_TIMEZONE
    value: "Asia/Tokyo"
```

## Development

```bash
# Build
go build -ldflags="-s -w -X main.version=$(git describe --tags --always)" -o ghacron main.go

# Run (dry-run)
GHACRON_APP_ID=123456 GHACRON_APP_PRIVATE_KEY="$(cat key.pem)" GHACRON_DRY_RUN=true ./ghacron

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
