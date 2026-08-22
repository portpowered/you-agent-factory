# Update pinned LocalAI backend artifacts

Status: maintainer procedure
Audience: Models, CI, release, and conformance maintainers

This procedure governs a LocalAI backend or protocol pin update. It keeps the
previous artifact set usable until the replacement set is complete and
conformant.

## Source of truth

The single pin input is
[`.github/localai-backend-artifacts.json`](../../../.github/localai-backend-artifacts.json).

The file contains these input groups:

| Input | Fields | Purpose |
| --- | --- | --- |
| LocalAI source | `localaiRepository`, `localaiCommit` | Identifies the immutable LocalAI checkout. |
| Protocol | `protocolPath`, `protocolRevision` | Identifies the pinned gRPC protocol blob. |
| Backend sources | `backends[].sourceRepository`, `backends[].sourceCommit`, `backends[].sourcePinVariable` | Identifies each backend source and its LocalAI Makefile pin. |
| Build tools | `grpcCommit`, `vcpkgCommit`, `toolchain`, `nodeVersion` | Identifies external build inputs and the validation runtime. |
| Workflow and host pins | `workflowPins`, `hostToolchain` | Pins GitHub Actions revisions and the native package/tool versions used on each runner. |
| Targets | `targets` | Defines the closed `darwin-arm64`, `linux-amd64`, and `windows-amd64` set. |

The workflow is
[`.github/workflows/localai-backend-artifacts.yml`](../../../.github/workflows/localai-backend-artifacts.yml).

The workflow validates the exact three-backend by three-target matrix. It also
checks out the pinned source, verifies the protocol blob, and checks each
backend Makefile pin before building.

The llama.cpp package contract follows the pinned LocalAI checkout: the source
Makefile is `backend/cpp/llama-cpp/Makefile`, its gRPC dependency is
`backend/cpp/grpc`, and the `../../grpc` recursive build path resolves to that
directory. The CPU package target produces `llama-cpp-cpu-all`; that is the
payload name recorded in the config and metadata. Linux uses the upstream
package target, Darwin stages the Mach-O binary and `.dylib` files without the
Linux-loader packaging branch, and Windows builds with the static
`x64-mingw-static` vcpkg triplet before recursively staging and verifying its
remaining DLL closure.

## Update procedure

1. Open a dependency PR for the replacement pin.

2. Change the required values in
   `.github/localai-backend-artifacts.json`.

3. Use immutable 40-character lowercase commits for source and dependency
   commit fields and immutable full-SHA GitHub Action revisions in
   `workflowPins`. Use exact numeric versions for toolchain and host package
   fields. Use the protocol blob SHA for `protocolRevision`.

4. Keep the backend and target identifiers unchanged. Do not add a backend,
   target, runner, or accelerator in this procedure.

5. Run the input and matrix checks from the repository root:

   ```sh
   node scripts/localai-backend-artifact-workflow.mjs validate \
     --config .github/localai-backend-artifacts.json
   node scripts/localai-backend-artifact-workflow.mjs matrix \
     --config .github/localai-backend-artifacts.json
   ```

   Confirm that validation prints
   `LOCALAI_BACKEND_ARTIFACT_INPUTS_OK combinations=9`.

   Do not replace the exact Linux, macOS, or MSYS2 host versions with floating
   package installs. The workflow verifies the Linux runner image, installs
   the macOS GNU Make formula from its pinned Homebrew commit, and requests
   the exact MSYS2 package versions from the manifest.

6. Run the fixture-backed manifest and compatibility checks:

   ```sh
   node --test scripts/localai-backend-artifact-workflow.test.mjs
   go test ./pkg/services/models/internal/artifacts
   go vet ./pkg/services/models/internal/artifacts
   ```

   These checks use fixture manifests and fake artifact bytes. They do not
   download model weights, start a backend, or connect to port `7437`.

7. Dispatch the reviewed workflow ref after the dependency PR is ready:

   ```sh
   gh workflow run .github/workflows/localai-backend-artifacts.yml \
     --ref <dependency-pr-ref>
   ```

   The workflow must complete all nine build jobs. A failed or cancelled job
   prevents the join and publication jobs from succeeding.

8. Review the generated publication bundle. The join job must produce exactly
   nine archives and one `manifest.json`.

   Each manifest entry must retain its backend source, LocalAI source,
   protocol, target, accelerator, archive location, byte size, and lowercase
   SHA-256. The join job reads every archive again before upload.

   Unix archives use `.tar.gz`. Windows archives use `.zip`. Each archive name
   follows `localai-backend-<backend>-<target>-<backendSourceCommit>.<extension>`.

9. Confirm the publication identity. The pin fingerprint covers the canonical
   pin document. The release tag has this form:
   `localai-backends-v1-<pinFingerprint>`.

   Archive names include the backend source commit and target. An existing
   release tag is immutable. The workflow refuses to overwrite it.

10. Wait for the C1 scheduled real-backend conformance run before adoption.
    C1 must cover `OMNI`, `EMBED`, `ASR`, and `TTS` on every supported target.

11. Merge and adopt the replacement only after the fixture checks, publication
    integrity checks, and C1 conformance all pass.

## Manifest and publication rules

The workflow generates `manifest.json` during the join job. The manifest is
not a hand-authored checksum file and is not edited in the dependency PR.
Treat the pin change and this generated manifest as one review unit.
The dependency PR is incomplete until it links its generated bundle or run
output for reviewer inspection.

The publication bundle contains the nine final archives plus the manifest.
Matrix provenance sidecars support validation and do not replace the manifest.
The publish job promotes the bundle only after the complete set is validated.

C1 may publish only backend version, model revision, operation, hardware class,
duration, and result digest. It must never publish model weights, prompts, or
model outputs.

Do not change Models selection, cache state, process supervision, protocol
clients, or TTS behavior in a pin-update PR. Those changes belong to their
own service or conformance lanes.

## Failure, rollback, and rerun

| Failure | Required response |
| --- | --- |
| Input or closed-matrix validation fails | Fix the dependency PR. Do not dispatch publication. |
| Source, protocol, toolchain, or payload verification fails | Keep the prior pin adopted. Do not use any produced archive. |
| A build is missing or cancelled | Do not promote a partial set. Rerun the full workflow after correcting the cause. |
| Join integrity or manifest validation fails | Treat the candidate as invalid. Never hand-edit a digest or size. |
| The release tag already exists | Keep the existing immutable release. Do not overwrite or redefine it. |
| C1 conformance fails | Keep the prior pin selected. Open a new dependency PR with a new pin fingerprint. |
| A transient CI failure occurs with unchanged inputs | Inspect the failure, then rerun the failed workflow once. |

The previous adopted release remains the rollback target until the replacement
passes C1. Rollback means restoring the previous manifest selection. It does
not mean mutating or deleting the previous release.

If a failed run leaves a draft release, do not promote it. Have the release
maintainer clean up the draft before rerunning the same immutable pin.

## Focused documentation check

This file is an internal maintainer how-to, not a packaged CLI reference topic.
Run the focused Markdown check after editing it:

```sh
go run ./cmd/markdown-linter docs/internal/howtos/pinned-localai-backend-artifacts.md
```
