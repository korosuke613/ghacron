---
name: generate-release
description: Determine the next semver from Conventional Commits and create a GitHub Release. Triggers GoReleaser workflow automatically.
argument-hint: "[tag e.g. v1.0.0 (optional)]"
---

GitHub Release を作成して。

## 手順

### 1. 前回リリースタグの特定

```
git tag --sort=-v:refname | head -5
```

タグが存在しない場合は初回リリースとして `v0.0.0` をベースラインとする。

### 2. Conventional Commits の解析

前回タグから HEAD までのコミットを取得し分類:

```
git log <prev-tag>..HEAD --oneline
```

- **Breaking Changes**: `!` 付き or `BREAKING CHANGE` フッター → **major** bump
- **Features**: `feat` → **minor** bump
- **Bug Fixes**: `fix` → **patch** bump
- **Other**: `refactor`, `perf`, `docs`, `ci`, `chore` など → バンプなし（他に feat/fix がなければ **patch**）

### 3. リリース種別の確認

$ARGUMENTS が指定されている場合はそれをタグとして使用し、このステップをスキップ。

未指定の場合、ユーザーにリリース種別を質問する:

- **Stable release** — 正式リリース（例: `v1.0.0`）
- **Release candidate** — RC（例: `v1.0.0-rc.1`）

### 4. 次バージョンの決定

Conventional Commits の解析結果とリリース種別から算出:

- 最も高い bump レベルを適用（major > minor > patch）
- 前回が `v0.x.y` の場合、breaking change は minor bump に留める（0.x 慣習）
- RC の場合は `-rc.N` サフィックスを付与。既に同バージョンの RC が存在する場合は番号をインクリメント（例: `-rc.1` → `-rc.2`）
- 算出したバージョンをユーザーに提示し、承認を得てからジッコウすること

### 5. リリースノート生成

カテゴリごとに英語で箇条書き:

```markdown
## What's Changed

### Breaking Changes
- ...

### Features
- ...

### Bug Fixes
- ...

### Other Changes
- ...
```

### 6. タグ作成とリリース

```
git tag v<version>
git push origin v<version>
gh release create v<version> --title "v<version>" --notes "<release-notes>"
```

- pre-release タグ（`-rc`, `-alpha`, `-beta`）の場合は `--prerelease` を付与
- GoReleaser ワークフロー（`.github/workflows/release.yml`）が `release: published` で自動起動し、バイナリ添付と Docker イメージビルドを行う

### 7. 確認

リリース作成後、ワークフロー起動を確認:

```
gh run list --workflow=release.yml --limit=3
```
