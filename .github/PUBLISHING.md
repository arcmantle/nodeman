# Publishing flow

nodeman release publishing is split into three automated paths:

1. GitHub Release + Homebrew formula updates via GoReleaser.
2. Winget manifest PR updates on every published GitHub release.
3. Chocolatey package publication on every published GitHub release.

## Required repository secrets

- `HOMEBREW_TAP_GITHUB_TOKEN`: PAT with push access to `arcmantle/homebrew-tap`.
- `WINGET_TOKEN`: classic PAT with `public_repo` scope used by Winget Releaser.
- `CHOCO_API_KEY`: Chocolatey API key for <https://push.chocolatey.org/>.

If a secret is not configured, the related publish step is skipped.

## Workflow files

- `.github/workflows/release.yml`
  - Trigger: push a `v*` tag.
  - Runs tests/vet.
  - Runs GoReleaser and publishes GitHub assets + checksums.
  - Updates Homebrew formula in `arcmantle/homebrew-tap` when `HOMEBREW_TAP_GITHUB_TOKEN` exists.

- `.github/workflows/publish-winget.yml`
  - Trigger: push a `v*` tag (and manual `workflow_dispatch`).
  - Publishes manifests using `vedantmgoyal9/winget-releaser`.

- `.github/workflows/publish-chocolatey.yml`
  - Trigger: push a `v*` tag (and manual `workflow_dispatch`).
  - Builds and pushes a `.nupkg` using `scripts/choco-pack-and-push.ps1`.

## Winget first submission bootstrap

Winget Releaser expects package identity metadata and generally works best once an
initial package exists in `microsoft/winget-pkgs`.

Recommended first-time flow:

1. Fork `microsoft/winget-pkgs` under the account/org that owns this repository.
2. Submit the first `Arcmantle.Nodeman` manifest manually.
3. Merge that PR.
4. After that, automated updates from `publish-winget.yml` can keep future versions current.
