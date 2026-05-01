# Development Guide Relevant Files

This inventory records the checked-in files and directories that the maintainer development guide should describe when it gives repository-root workflow instructions.

| Path | Role | Notes |
| --- | --- | --- |
| `go.mod` | Repository-root marker | Maintainer commands and worktree-aware tests should treat the directory containing `go.mod` as the canonical repository root. |
| `Makefile` | Root command surface | The development guide should describe quality and generation commands as root-level invocations instead of teaching a nested package workflow. |
| `api/` | Authored API contract workspace | OpenAPI validation and bundling start from the repository root, then shell into `api/` only where the documented workflow requires it. |
| `api/components/schemas/shared/` | Shared OpenAPI fragment family | Cross-surface helper schemas such as maps and diagnostics live here and referenced fragments must use file-relative `$ref`s. |
| `api/components/schemas/runtime/` | Runtime OpenAPI fragment family | Request, response, status, and work-submission contract fragments are authored here one schema per file. |
| `api/components/schemas/factory-config/` | Factory-config OpenAPI fragment family | Public named-factory and factory-topology schemas are authored here and should stay split from `api/openapi-main.yaml`. |
| `api/components/schemas/factory-world/` | Additive factory-world OpenAPI fragment family | Dashboard-facing read-model schemas belong here one schema per file rather than remaining inline in `api/openapi-main.yaml`. |
| `cmd/factory/` | CLI entrypoint | Root-level build and smoke commands compile or execute the `factory` binary from this source tree. |
| `docs/development/*-closeout.md` | Cleanup verification artifacts | Narrow cleanup lanes record the exact root-level validation bundle here when maintainers need durable proof beyond `progress.txt`. |
| `docs/development/openapi-schema-standardization-inventory.md` | OpenAPI cleanup inventory | Records the authored fragment layout, remaining inline schemas, and the canonical bundle and generation verification surfaces for schema-standardization work. |
| `docs/development/root-factory-artifact-contract-inventory.md` | Root artifact contract inventory doc | The checked-in root artifact table must stay in lockstep with the enforced entries in `internal/testpath/artifact_contract.go`. |
| `docs/development/development.md` | Active maintainer guide | Must describe the real repository-root layout used in this checkout and avoid stale `libraries/agent-factory` instructions. |
| `.github/workflows/ci.yml` | Repository CI contract | Contributor docs should name this workflow and mirror its root-level validation commands and stated non-deployment scope. |
| `internal/testpath/artifact_contract.go` | Enforced root artifact contract | Root-level factory artifact additions, removals, and redirect stubs are test-enforced here and must stay synchronized with the inventory doc. |
| `factory/` | Maintainer workflow surface | Contains checked-in operator guidance and active inbox directories that the development guide may reference for workflow-related tasks. |
| `pkg/api/handlers.go` | Handwritten API decode and validation boundary | Unsupported or legacy public request fields for `POST /work` still need explicit raw-JSON rejection here because generated request structs do not reject unknown keys by themselves. |
| `pkg/api/openapi_contract_test.go` | OpenAPI contract guard surface | Focused authored-versus-bundled contract assertions live here, including fragment-layout and `/events` schema wiring checks. |
| `pkg/api/testdata/canonical-event-vocabulary-stream.json` | OpenAPI vocabulary fixture | Canonical bundled-contract fixtures for event payload validation live here and must be updated alongside public schema field renames. |
| `pkg/interfaces/public_factory_enums.go` | Shared public factory enum normalizers | Shared worker and workstation alias ownership lives here; exported callers must choose explicit strict or permissive helpers instead of duplicating alias tables. |
| `pkg/interfaces/factory_runtime.go` | Handwritten public work-request boundary structs | Watched-file batch ingestion, generated worker output parsing, and fixture helpers still marshal these structs directly, so their JSON tags must stay aligned with the camelCase OpenAPI contract. |
| `pkg/workers/provider_behavior.go` | Provider CLI behavior owner | Provider-specific argument construction and command-request shaping live here; worker inference paths should call this seam instead of reintroducing forwarding wrappers in `inference_provider.go`. |
| `pkg/` | Go implementation surface | Package-specific test commands in the guide should reference the real package paths under this root. |
| `tests/` | Smoke and fixture surface | Functional and release-facing checks run from the repository root against these checked-in fixtures. |
| `tests/functional_test/` | Behavioral regression suite | Functional tests should prove observable runtime, API, CLI, dashboard, or emitted-event behavior; source scans, docs topology checks, bundle internals, and command or route inventories are maintenance guards rather than product-facing smoke coverage. |
| `ui/` | Embedded dashboard workspace | UI build, test, and Storybook commands remain part of the same repository-root workflow. |
| `ui/src/testing/replay-fixture-catalog.ts` | Replay integration test contract | Browser-backed dashboard smoke coverage should register scenario metadata here so coverage reporting and integration assertions stay on one source of truth. |
| `ui/scripts/write-replay-coverage-report.ts` | Replay coverage reporter | Package scripts should use this repository-owned reporter to validate replay metadata instead of embedding ad hoc fixture maps in tests or CI. |
| `ui/scripts/normalize-dist-output.mjs` | Embedded asset normalizer | The documented UI build path ends by normalizing Vite output names and refreshing `ui/dist_stamp.go` so committed embed assets stay stable for Go builds and CI diffs. |

## Reusable Rules

- When maintainer docs describe command execution, anchor the instructions to the repository root that contains `go.mod` and `Makefile`.
- If a workflow temporarily changes directories, state that it starts from the repository root and why the subdirectory hop is required.
- When GitHub Actions or other automation is added, prefer repository-owned root commands or package scripts that the maintainer guide already documents instead of inventing CI-only command sequences.
- When contributor docs mention the repository CI workflow, mirror the exact root-level command sequence and its stated scope from `.github/workflows/ci.yml` so local reproduction and review expectations do not drift.
- When UI assets are committed for Go embedding, keep the build pipeline responsible for normalizing output filenames and refreshing any cache-busting stamp files instead of hand-editing `ui/dist/`.
- When browser-backed UI replay tests and replay coverage reports share the same scenarios, keep that metadata in one repository-owned catalog so the tests, scripts, and docs cannot silently drift.
- When GitHub Actions uses `actions/setup-go` against a module that declares a newer `toolchain` than its base `go` version, prefer uncached setup unless you have verified that cache restore does not collide with the auto-downloaded toolchain files in later jobs.
- When a cleanup lane closes with path or contract-alignment work, record the exact root-level verification commands in a `docs/development/*-closeout.md` artifact so the proof survives beyond `progress.txt`.
- When functional tests need `os.Chdir`, route the directory swap through the shared helper and serialize it with a package-level lock; process working directory is global state and will race any `t.Parallel()` coverage otherwise. If the same test changes directories more than once or does so inside subtests, use the scoped helper form so the lock is released between command executions instead of being held until parent-test cleanup.
- Keep `tests/functional_test/` focused on observable behavior. If a check only proves source layout, docs linkage, asset bundle contents, or command or route registration, move it to a narrower guard surface instead of adding it to the functional suite.
- When a functional test covers `agent-factory docs`, run the command from a temp working directory without a local docs tree and assert user-visible headings or stable content markers rather than byte-for-byte parity with packaged markdown files.
- When `make lint` fails in `cmd/deadcodecheck`, remove orphaned `_test.go` helper files and unused test wrappers before touching `docs/development/deadcode-baseline.txt`; the baseline is for accepted remaining debt, not abandoned local helpers.
- Use `pkg/api/openapi_contract_test.go` for narrow OpenAPI contract guards when the work is about authored schema structure or bundled route/schema alignment rather than handler runtime behavior.
- Keep `docs/development/root-factory-artifact-contract-inventory.md` and `internal/testpath/artifact_contract.go` synchronized; the doc is not descriptive-only, it is a checked-in contract surface with order-sensitive tests.
- When public OpenAPI field names change, update `pkg/api/testdata/canonical-event-vocabulary-stream.json` together with the contract guards so fixture validation keeps exercising the current bundled vocabulary.
- When public request-batch field names change, update `pkg/interfaces/factory_runtime.go`, `pkg/factory/work_request_json.go`, and watched-file/worker batch fixtures together; those handwritten JSON boundaries are not generated and will drift silently if only `api/openapi.yaml` and generated clients are regenerated.
- Keep shared public worker and workstation enum aliases in `pkg/interfaces/public_factory_enums.go`; generated/OpenAPI boundary normalizers should call the strict helpers while generated/public round-trip helpers should call the permissive ones instead of cloning alias tables in `pkg/config` or UI export code.
- `POST /work` is the single-submit surface and should reject relation graphs explicitly; named relation wiring belongs on `PUT /work-requests/{requestId}`, where the handler can resolve `sourceWorkName` and `targetWorkName` safely.
- `decodeSubmitWorkRequestBody` is the handwritten validation seam for `POST /work`; use the raw JSON field map there whenever generated structs need compatibility checks beyond Go's default `json.Unmarshal` behavior, especially when unary submit fields differ from canonical batch fields.
- For Redocly-bundled vendor extensions that must keep a schema pointer in `api/openapi.yaml`, store the extension value as a string JSON Pointer; nested `$ref` objects under `x-*` fields are inlined during bundling.
- When dashboard or world-view code needs runtime-only observability that is not reconstructed from canonical events, keep the event reducer unchanged and overlay the live engine snapshot fields at the projection boundary that already owns both inputs.
- For dispatcher pause-gating tests, use scheduler doubles that derive decisions from the `enabled` slice they receive so the assertions reflect the real FIFO/work-queue scheduler contract instead of depending on impossible post-selection decisions.
- When `make lint` fails on `cmd/deadcodecheck`, remove truly stale symbols first and only then refresh `docs/development/deadcode-baseline.txt` for the remaining accepted library or test-helper debt in the same review.
- Keep provider-specific CLI argument assembly and command-request shaping in `pkg/workers/provider_behavior.go`; `pkg/workers/inference_provider.go` should assemble execution around that seam rather than growing duplicate forwarding helpers.
- Keep provider CLI build-args regression tests adjacent to `pkg/workers/provider_behavior.go`; `inference_provider` tests should stay focused on command assembly and execution behavior instead of duplicating provider flag assertions through wrapper-oriented seams.
- When `inference_provider` tests need to verify provider command payloads, derive the expected args and command request from `providerBehaviorFor(...).BuildArgs(...)` and `BuildCommandRequest(...)` so the regression fails if orchestration drifts away from the canonical provider-behavior owner.
