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

## Configuration

Configure via `config/config.yaml`:

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

webapi:
  enabled: true
  host: "0.0.0.0"
  port: 8080
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

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/healthz` | GET | Health check |
| `/status` | GET | Service status (registered cron job count, etc.) |
| `/jobs` | GET | List of registered cron jobs |
| `/config` | GET | Public configuration info |

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
