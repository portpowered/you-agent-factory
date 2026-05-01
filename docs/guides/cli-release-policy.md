# CLI Release Policy

This guide defines the release policy for the `cmd/factory` CLI. Use it as the
single operator workflow for versioned releases.

## Release Model

- Releases are cut from `main`.
- Maintainers create a manual semver tag such as `v1.2.3` after the target
  commit is already on `main`.
- Only pushed tags matching `v*` are allowed to trigger release publication in
  GitHub Actions.
- Phase one release outputs are GitHub release archives, checksums, and the
  Homebrew cask metadata generated from those same release assets.

## Why Tag On `main`

Tagging the exact `main` commit keeps one audited release path:

- The reviewed pull request merge commit is the release input, so there is no
  separate release-branch drift to manage.
- The same GitHub Actions release workflow can validate and publish from one
  immutable ref instead of coordinating branch merges plus a second release
  event.
- Manual GitHub Release creation is not the source of truth. The tag is the
  source of truth, and GitHub Actions owns publication from that tag.

This repository should not use long-lived release branches for normal CLI
releases unless a future PRD changes the release model. Release branches add
another stateful surface to synchronize and make it easier for the published
artifact to diverge from the already-reviewed `main` history.

This repository should not use manually created GitHub Release events as the
publication trigger. They are easier to fire from the wrong commit and harder
to reproduce locally than a visible semver tag on `main`.

## Standard Release Flow

Use this sequence for every CLI release:

1. Merge the release-ready change set into `main`.
2. Update your local checkout and confirm `main` points at the reviewed commit.
3. Run `make release VERSION=v0.4.0` from a clean `main` checkout.
4. Let the command run the local readiness checks, create the semver tag, and
   push only that tag to `origin`.
5. Watch the tag-triggered GitHub Actions release workflow for candidate
   verification, artifact publication, and post-publish verification.
6. Confirm the GitHub release contains the expected archives and checksums for
   Windows, Linux, and macOS.

## Installation Surfaces Published By Release

The release workflow publishes one set of GitHub release archives and reuses
that output for every supported installation path:

- GoReleaser builds the tagged `agent-factory` archives and checksum file from
  `.goreleaser.yml`.
- The same GoReleaser run updates the `portpowered/cask` tap on `main` by
  writing the generated cask file to `Casks/agent-factory.rb`.
- The publish workflow then uploads the repo-owned `install.sh` from the tagged
  commit as a GitHub release asset, so the hosted installer URL becomes:

```text
https://github.com/portpowered/infinite-you/releases/download/vX.Y.Z/install.sh
```

For consumers, the expected Homebrew install command remains:

```bash
brew install --cask portpowered/cask/agent-factory
```

The latest-release fallback installer path remains:

```bash
curl -fsSL https://github.com/portpowered/infinite-you/releases/latest/download/install.sh | sh
```

Those install surfaces must keep pointing at the same tagged GitHub release.
Do not publish a separate Homebrew-only build, a hand-edited cask, or a
different installer artifact path.

## Homebrew Tap Setup

Before the first automated cask publication can succeed, maintainers must
prepare the tap and credentials expected by `.goreleaser.yml` and
`.github/workflows/release.yml`:

- The Homebrew tap repository is `portpowered/cask`.
- The generated cask is committed on the tap's `main` branch under
  `Casks/agent-factory.rb`.
- GitHub Actions needs a `HOMEBREW_TAP_GITHUB_TOKEN` secret with permission to
  push to `portpowered/cask`.
- The token must be available to the release publish workflow because
  GoReleaser reads it from the `HOMEBREW_TAP_GITHUB_TOKEN` environment variable
  declared in `.goreleaser.yml`.

If the tap repository layout, branch, or secret name changes, update both the
GoReleaser config and the release workflow in the same review. The maintainer
guide, workflow, and config must continue to describe one identical tap
publication contract.

## Supported `go install` Path

The release process must preserve `cmd/factory` as the stable installable Go
entrypoint.

For Go users, the supported command is:

```bash
go install github.com/portpowered/infinite-you/cmd/factory@latest
```

This install path is for environments that already have a working Go toolchain.
General consumers should prefer the packaged release surfaces instead of
building from source through `go install`.

`make release` is the maintainer-owned release-preparation command. It fails
fast when:

- `VERSION` is missing or does not match `vMAJOR.MINOR.PATCH`.
- The current branch is not `main`.
- The working tree is dirty.
- The tag already exists locally or on `origin`.

The command does not publish artifacts from the developer machine. It runs the
repository readiness checks, then pushes the tag so GitHub Actions remains the
only publication path.

## Example

Example release cut for `v0.4.0`:

```bash
git checkout main
git pull --ff-only origin main
make release VERSION=v0.4.0
```

After the push:

- GitHub Actions should detect the `v0.4.0` tag.
- The release workflow should ignore non-semver branch pushes for publication.
- Maintainers should monitor the workflow until the release assets and checksums
  are available on the GitHub release page.
- Homebrew consumers should be able to install the published cask through
  `brew install --cask portpowered/cask/agent-factory`.
- The hosted installer should be reachable from the tag-specific release asset
  URL and the latest-download URL after the asset upload step completes.
- Release verification should keep one repo-owned `go install ./cmd/factory`
  smoke step so the stable CLI entrypoint remains buildable into a clean
  `GOBIN` before maintainers rely on the documented public command.

## Release Failure Triage

Interpret post-publish failures by the job that reported them:

- `Publish GitHub Release` failures usually mean GoReleaser could not build or
  upload the tagged archives, checksums, or Homebrew cask update. Check the
  GoReleaser logs first, then confirm the tag points at the intended commit and
  that `HOMEBREW_TAP_GITHUB_TOKEN` is present with push access to
  `portpowered/cask`.
- `Verify Homebrew Cask Publication` failures mean the tap checkout or the
  generated `tap/Casks/agent-factory.rb` content is wrong for the tagged
  release. Confirm the cask version, asset URL, checksum, and install behavior
  match the published release artifacts.
- `Smoke Hosted install.sh` failures mean the uploaded `install.sh` asset or
  its runtime archive-selection and checksum logic is wrong. Verify the release
  contains the expected `install.sh` asset, that the installer URL resolves,
  and that the referenced archive and checksum files exist for the target
  platform.
- `Verify Go Install Surface` failures mean the source-install contract for
  `cmd/factory` regressed. Reproduce with the focused `tests/release`
  `go install ./cmd/factory` smoke test before changing consumer docs or the
  release workflow.
