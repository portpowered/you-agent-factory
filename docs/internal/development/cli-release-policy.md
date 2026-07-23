# Release Policy

This repository releases from a maintainer-created semantic-version tag on
`main`. Maintainers run `make release VERSION=vX.Y.Z` from a clean `main`
checkout; that command verifies readiness and pushes only the tag. GitHub
Actions owns every publication side effect.

## Data package candidates

`@you-agent-factory/api` and
`@you-agent-factory/packaged-factories` use one candidate identity policy. A
candidate version has the form
`0.0.0-dev.<workflow-run-id>.<first-12-source-commit-characters>`. Candidate
evidence and the staged contract manifest record the same full source commit.
The checked-in package versions remain the `0.0.0` staging placeholder.

The Packaged Factories package is data-only. Its supported npm exports are:

- `/manifest`
- `/schemas/factory.json` and `/schemas/factory.yaml`
- `/factories/<manifest-slug>.json` and
  `/factories/<manifest-slug>.yaml`

The manifest is the Factory inventory. Authored Factory directories, prompts,
scripts, and package-internal paths are not public npm exports.

Use these focused commands from the repository root:

```bash
make packaged-factory-catalog-generate
make packaged-factory-catalog-check
make packaged-factory-package-script-test
make packaged-factory-package-pack-check
make packaged-factory-package-candidate-dry-run
make packaged-factory-package-consumer-smoke
```

The pack check replaces
`.artifacts/packaged-factories-local-pack`; the dry-run and consumer commands
replace `.artifacts/packaged-factories-local-dry-run`. The dry-run is
publication-free and uses the current full commit as both the candidate source
and simulated reviewed head. It preserves the exact tarball, candidate
evidence, and installed-consumer evidence.

## Protected publication

Pull-request jobs can generate, pack, install, verify, and preserve candidates,
but they have no npm publication step or publication credentials. The
Development Package workflow prepares separate API and Packaged Factories
artifacts from the reviewed pull-request head. A protected `main` run prepares
and uploads each candidate before a later job downloads and publishes it.

The tagged Release workflow follows the same boundary. One job prepares both
data-package candidates from
`github.event.workflow_run.head_sha` and preserves them under distinct artifact
names. The publication job downloads those candidates, requires their evidence
to name that exact source commit, and never repacks mutable checkout sources.
Trusted npm publication uses provenance and the candidate's `dev` distribution
tag. The frontend package family retains its separate release-version and
`latest`-tag policy.

## Reconciliation and recovery

Publication verifies candidate evidence and the local tarball digest before
registry lookup:

- `PUBLISH_REQUIRED` means the immutable version is absent and may be published
  once.
- `VERIFIED_EXISTING` means the registry already contains byte-identical
  content; the workflow succeeds without republishing or moving a tag.
- `PUBLISHED_AND_VERIFIED` means publication completed and the exact registry
  version passed clean-consumer verification.

Registry lookup, publication, visibility polling, and installed-consumer
verification are bounded. Authentication and permission failures require
repairing npm trusted-publishing configuration before rerunning the protected
workflow. Lookup, download, publication, or timeout failures may be retried
after checking npm availability; reconciliation makes an accepted prior publish
safe to resume. A candidate digest failure, source/version mismatch, or
`IMMUTABLE_VERSION_CONFLICT` is not retryable drift: stop, preserve the
evidence, and investigate the reviewed candidate or registry bytes. Never
overwrite an existing version or manually move its distribution tag to hide a
mismatch.
