# CLI Release Policy

This guide defines the release policy for the `cmd/factory` CLI. Use it as the
single operator workflow for versioned releases.

## Release Model

- Releases are cut from `main`.
- Maintainers create a manual semver tag such as `v1.2.3` after the target
  commit is already on `main`.
- Only pushed tags matching `v*` are allowed to trigger release publication in
  GitHub Actions.
- Phase one release outputs are GitHub release archives and checksums only.
  Package-manager publishing stays out of scope until a later story changes the
  policy explicitly.

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
3. Create the semver tag from `main`, for example `git tag v0.4.0`.
4. Push the tag with `git push origin v0.4.0`.
5. Watch the tag-triggered GitHub Actions release workflow for candidate
   verification, artifact publication, and post-publish verification.
6. Confirm the GitHub release contains the expected archives and checksums for
   Windows, Linux, and macOS.

## Example

Example release cut for `v0.4.0`:

```bash
git checkout main
git pull --ff-only origin main
git tag v0.4.0
git push origin v0.4.0
```

After the push:

- GitHub Actions should detect the `v0.4.0` tag.
- The release workflow should ignore non-semver branch pushes for publication.
- Maintainers should monitor the workflow until the release assets and checksums
  are available on the GitHub release page.
