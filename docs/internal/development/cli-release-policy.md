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
  hosted installer assets generated from that same tagged release.

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

- GoReleaser builds the tagged `infinite-you` archives and checksum file from
  `.goreleaser.yml`.
- The publish workflow then uploads the repo-owned `scripts/install.sh` from the tagged
  commit as a GitHub release asset, so the hosted installer URL becomes:

```text
https://github.com/portpowered/infinite-you/releases/download/vX.Y.Z/install.sh
```

- The publish workflow also uploads the repo-owned `scripts/install.ps1` from the
  tagged commit as a GitHub release asset for Windows installs:

```text
https://github.com/portpowered/infinite-you/releases/download/vX.Y.Z/install.ps1
```

The latest-release fallback installer path remains:

```bash
curl -fsSL https://github.com/portpowered/infinite-you/releases/latest/download/install.sh | sh
```

For Windows PowerShell consumers, the latest-release fallback installer path
remains:

```powershell
irm https://github.com/portpowered/infinite-you/releases/latest/download/install.ps1 | iex
```

Those install surfaces must keep pointing at the same tagged GitHub release.
Do not publish a separate package-manager-only build or a different installer
artifact path that bypasses the tagged archive and checksum flow.

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
- The hosted installers should be reachable from the tag-specific release asset
  URLs and the latest-download URLs after the asset upload step completes.
- Release verification should keep both a repo-owned `go install ./cmd/factory`
  smoke step and an outside-the-repo public-module smoke of
  `go install github.com/portpowered/infinite-you/cmd/factory@latest` so the
  stable entrypoint stays buildable locally and the documented consumer command
  is exercised against the published module path on release.

## Release Failure Triage

Interpret post-publish failures by the job that reported them:

- `Publish GitHub Release` failures usually mean GoReleaser could not build or
  upload the tagged archives or checksums. Check the GoReleaser logs first,
  then confirm the tag points at the intended commit and the workflow still
  has the expected GitHub release permissions.
- `Smoke Hosted Installer` failures mean the uploaded `install.sh` or
  `install.ps1` asset, or its runtime archive-selection and checksum logic, is
  wrong. Verify the release contains the expected installer assets, that the
  installer URLs resolve, and that the referenced archive and checksum files
  exist for the target platform.
- `Verify Go Install Surface` failures mean the source-install contract for
  `cmd/factory` regressed. Reproduce first with the focused repo-owned
  `tests/release` `go install ./cmd/factory` smoke, then confirm the public
  `go install github.com/portpowered/infinite-you/cmd/factory@latest` path
  still works against the published module when the release workflow reports a
  post-publish failure.
