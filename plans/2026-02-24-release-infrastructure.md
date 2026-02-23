# リリースインフラ整備

## Context

ghacron は現在バージョンが `main.go` にハードコードされ、Git タグもなく、バイナリ配布手段がない。
Conventional Commits 導入済みの今、リリースフローを整備する:
- ldflags によるバージョン注入（ハードコード廃止）
- GoReleaser で linux/amd64+arm64 バイナリを GitHub Release に添付
- Docker イメージビルドへのバージョン伝播
- 初期は `/generate-release` スキルで手動リリース。将来的に Claude Code Action で自動化

## 変更対象ファイル

### 1. `main.go` — バージョン変数化

```go
// Before
const Version = "0.1.0"

// After
var version = "dev"
```

- `const` → `var` で ldflags `-X main.version=...` による注入を可能に
- エクスポート不要のため小文字に変更
- 参照箇所（L29, L39）も `version` に更新

### 2. `.goreleaser.yml` — 新規作成

```yaml
version: 2
project_name: ghacron

before:
  hooks:
    - go mod tidy

builds:
  - id: ghacron
    main: ./main.go
    binary: ghacron
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X main.version={{.Version}}

archives:
  - id: ghacron
    builds:
      - ghacron
    format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

changelog:
  disable: true

release:
  github:
    owner: korosuke613
    name: ghacron
  draft: false
  prerelease: auto
```

- `changelog.disable: true` — リリースノートは `/generate-release` で手動作成
- Docker は GoReleaser の管轄外（既存ワークフローが担当）

### 3. `Dockerfile` — VERSION ビルド引数追加

```dockerfile
FROM golang:1.25-alpine AS builder

ARG VERSION=dev    # ← 追加

# ... 既存のまま ...

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o ghacron main.go
```

### 4. `.github/workflows/release.yml` — GoReleaser ジョブ追加 + Docker マルチアーチ分離

#### GoReleaser ジョブ（単一ランナー、クロスコンパイル）

```yaml
  goreleaser:
    if: github.event_name == 'release'
    runs-on: ubuntu-24.04
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

#### Docker マルチアーチ — ネイティブランナー分離戦略

既存の単一ランナー（arm64）を、amd64/arm64 ネイティブ並行ビルド + マニフェスト統合に分離する。
QEMU エミュレーションより高速かつ安定。

```yaml
  build-and-push:
    strategy:
      matrix:
        include:
          - runner: ubuntu-24.04       # amd64 ネイティブ
            platform: linux/amd64
          - runner: ubuntu-24.04-arm   # arm64 ネイティブ
            platform: linux/arm64
    runs-on: ${{ matrix.runner }}
    permissions:
      contents: read
      packages: write
    steps:
      # ... checkout, login, version判定 ...
      - name: Build and push (per-platform)
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          platforms: ${{ matrix.platform }}
          tags: <platform-specific digest tags>  # 後述
          build-args: VERSION=${{ steps.version.outputs.value }}
          cache-from: type=gha,scope=${{ matrix.platform }}
          cache-to: type=gha,scope=${{ matrix.platform }},mode=max

  merge-manifests:
    needs: build-and-push
    runs-on: ubuntu-24.04
    permissions:
      packages: write
    steps:
      # docker buildx imagetools create で amd64+arm64 ダイジェストを
      # 単一マルチアーチマニフェスト (latest, semver タグ) に統合
```

**構成:**
- `build-and-push`: matrix で amd64/arm64 を並行ビルド。各プラットフォーム固有のダイジェストを push
- `merge-manifests`: `docker buildx imagetools create` でマルチアーチマニフェストを生成
- `goreleaser` と Docker ジョブ群は並行実行（依存関係なし）
- push to main 時は `goreleaser` ジョブのみ `if` でスキップ

### 5. `CLAUDE.md` — Production Build コマンド更新

```bash
go build -ldflags="-s -w -X main.version=$(git describe --tags --always)" -o ghacron main.go
```

### 6. `.gitignore` — `dist/` 追加

GoReleaser のローカル実行で生成される `dist/` ディレクトリを除外。

## 検証

```bash
# 1. ldflags 注入確認
go build -ldflags="-s -w -X main.version=test" -o ghacron main.go && ./ghacron --version
# → "ghacron vtest"

# 2. デフォルト値確認
go build -o ghacron main.go && ./ghacron --version
# → "ghacron vdev"

# 3. GoReleaser dry-run
goreleaser release --snapshot --clean
# → dist/ に ghacron_*_linux_{amd64,arm64}.tar.gz が生成される

# 4. テスト
go test ./...
```
