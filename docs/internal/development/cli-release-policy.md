# Release Policy

This repository releases from a maintainer-created semantic-version tag on
`main`. Maintainers run `make release VERSION=vX.Y.Z` from a clean `main`
checkout; that command verifies readiness and pushes only the tag. GitHub
Actions owns every publication side effect.

## Public package sets

Development publication covers only the five-package frontend family:
`@you-agent-factory/client`, `@you-agent-factory/factory-replay`,
`@you-agent-factory/factory-emulator`, `@you-agent-factory/components`, and
`@you-agent-factory/factory-visualizers`. Each protected-main candidate uses
the immutable version
`0.0.0-dev.<workflow-run-id>.<reviewed-source-commit>`. Pull-request runs
prepare the same frontend-only shape without publishing it.

Tagged releases use one complete seven-package set. They add
`@you-agent-factory/api` and `@you-agent-factory/packaged-factories` to the
frontend family, and every tarball uses the requested stable release version.
Candidate preparation stages patched manifests in temporary directories; the
checked-in package versions remain the `0.0.0` staging placeholder.

To prepare the complete tagged-release candidate locally without publishing,
run this from the repository root with an empty output directory:

```bash
node scripts/public-release-package-candidate.mjs \
  --output-directory .artifacts/public-release-candidate \
  --run-id 1 \
  --source-commit "$(git rev-parse HEAD)" \
  --version 1.2.3
```

The command writes the seven tarballs and their evidence beneath
`.artifacts/public-release-candidate`. It does not contact the npm publication
endpoint.

## Packaged Factories contract

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
Development Package workflow prepares only the frontend family from the
reviewed pull-request head. A protected `main` run preserves that exact
frontend-only candidate before a later job publishes it with provenance under
the `dev` distribution tag. API and Packaged Factories artifacts are not part
of development candidate preparation or publication.

The tagged Release workflow prepares the complete seven-package set from
`github.event.workflow_run.head_sha` and preserves it as one artifact. The
publication job requires the top-level and child evidence to name that exact
source commit and release version, and it never repacks mutable checkout
sources. Trusted npm publication uses provenance and the `latest` distribution
tag.

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
