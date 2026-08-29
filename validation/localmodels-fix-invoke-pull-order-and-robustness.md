# Validation report: localmodels-fix-invoke-pull-order-and-robustness

## Environment and artifact

- Commit/build identifier: `d65b8ed220cbef9383532346534f730b69b90748`; the
  Models integration package's `TestMain` built `./cmd/factory` once with
  `go build -buildvcs=false` and reused that delivered artifact for its
  process probes.
- Environment and configuration: Windows `amd64`, Go `go1.25.0`, Windows
  build `10.0.26200.0`; each probe used a fresh temporary home, cache, and
  factory. The source checkout was clean before validation, excluding ignored
  factory scaffolding.
- Customer entry point: built `you` Models invoke/server commands through
  `tests/integration/models`.
- Real and substituted dependencies: production wiring and local Windows
  processes/filesystem were real; the asset origin was a controlled local
  HTTP fixture selected by `YOU_MODELS_BACKEND_ENDPOINT`; no remote or paid
  provider was contacted.
- Cost/call budget used: `$0`, zero paid calls, zero remote calls, and no use
  of port `7438`.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Cost-safe preflight and ordered acquisition | PASS | `TestStory002ProvesAssetPreflightAndBodyOrder` passed in the clean package run: exact estimate `backendBytes=25 modelBytes=23 totalBytes=48`; backend content preceded model content. The focused preflight suite also passed the zero-body HEAD, backend-failure-before-model-metadata, and overflow-before-network assertions. | Real backend-tag availability and remote network behavior. |
| Typed failed pull and same-process survival | PASS | `TestStory003FailedPullReturnsTypedResponseAndServerSurvives` passed: HTTP 500, non-empty 92-byte typed JSON, unchanged PID, `/status` 200, and a later `/models` request 200. | A remote or production-hosted origin. |
| Five deterministic failing invokes | PASS | `TestStory004RendersFiveFailingInvokesDeterministically` passed: all five exited 1, stdout was 0 bytes, stderr was 541 bytes with the same SHA-256, code `MODEL_ASSET_PREPARATION_FAILED`, non-empty message, and identical debug SHA-256; JSON mode kept diagnostics on stderr. | Closed or unwritable operating-system stderr. |
| Cross-process staging and retry recovery | PASS | `TestStory005SerializesOverlappingBuiltInvokes` passed with `modelStarts=1`, both exits 0, no sharing/collision marker, and no partial cache entry. `TestStory005RecoversAfterOwnerExit` passed with retry exit 0, no collision, and no partial cache state. | Network-filesystem locking semantics. |
| CASE-001 through CASE-020 and named quality gates | BLOCKED | Focused Models tests, the race-qualified built-process subset, `go vet`, `make cli-manifest-check`, and `make cli-contract-smoke` exited 0. `make lint` exited 1: UI lint/deadcode tools are absent, `pkg-maint` reports the changed staging test at complexity 17 (limit 15), two package-boundary findings are pre-existing, and deadcode baseline counts drift (3130 baseline versus 3129 current). | A final all-green repository lint gate requires the delta plan below. |
| Security, privacy, and forbidden-port behavior | PASS | The controlled fixture and isolated environments produced zero remote/paid calls; diagnostic assertions cover secret, body, prompt, cache, and path redaction. No request was made to port 7438. | Network monitoring outside the test process. |
| LOOP-001 report and no-silent-repair rule | BLOCKED | This report was emitted from the delivered Windows run and records the failed gate plus a delta plan. No implementation file was changed after the gate failure. | Re-run after the required quality-gate correction. |

## Customer journey

1. The delivered source at `d65b8ed220` was validated on Windows `amd64` with
   isolated temporary home/cache/factory roots and a controlled local origin.
   The package build was performed once by `TestMain`; process probes reused
   that binary.
2. `go test ./tests/integration/models -short -count=1 -timeout 45m -v`
   exited 0 in 36.301s. The same run passed the ordered acquisition,
   server-survival, five-repeat, overlap, and owner-exit-retry witnesses.
3. The asset ledger observed backend HEAD and model metadata before content,
   then one backend content response of 25 bytes before one model content
   response of 23 bytes. The emitted missing-asset estimate was
   `models asset estimate modelName="embed" backendBytes=25 modelBytes=23 totalBytes=48`.
4. The failed pull returned typed JSON (`status=500`, `bodyBytes=92`), while
   the server PID stayed unchanged and both health and subsequent model-list
   requests returned 200.
5. The five failing invokes each returned exit 1 with the stable typed code,
   message, and debug chain described in the criteria table. Two overlapping
   invokes transferred the model once; killing the owner and retrying produced
   a usable completed cache with no partial artifact.
6. The race-qualified built subset exited 0 in 15.599s. The preflight edge
   suite exited 0 in 0.068s. `make cli-manifest-check` and
   `make cli-contract-smoke` both exited 0. `go vet` exited 0.
7. `make lint` exited 1 and ended the clean-room validation at the quality
   gate. The implementation was not edited to disguise or repair that result.

## Cross-task integration and usability

- Documentation discoverability: this report is at
  `validation/localmodels-fix-invoke-pull-order-and-robustness.md`; the
  executable witnesses remain in `tests/integration/models`.
- Permission and error behavior: typed pull and invoke failures are non-empty,
  stable, and request/process scoped; sensitive wrapped causes are redacted.
- Persistence/reload behavior: completed asset metadata and content survived
  the overlap and owner-exit retry probes; no partial entry was ready.
- Accessibility/keyboard/responsive behavior: not applicable; this lane has
  no UI change.
- Operational signals: exit status, bounded stdout/stderr sizes and hashes,
  request/body-byte ledger, PID, health status, transfer count, cache
  completeness, and race/vet/gate exit status were captured.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| VAL-001 | Blocker | Run `make lint` at commit `d65b8ed220`. | All required repository lint targets pass. | Five targets fail: missing UI binaries, one changed staging test exceeds the maintainability threshold, two unrelated test-only boundary findings, and deadcode baseline drift. | `make lint` output and the exact `pkg-maint` finding for `TestPrepareGenericAssetsRecoversAfterStagingAccessDenied`. |

## Verdict

BLOCKED

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: Story 006 quality-gate audit and the
  overall named `make lint` acceptance criterion.
- Root-cause evidence or remaining uncertainty: `pkg-maint` identifies a
  complexity-17 test introduced by the staging work; UI lint/deadcode cannot
  start because `@biomejs/biome` and `knip` are unavailable; the two
  package-boundary findings and deadcode count drift are outside this lane's
  Models surface. The four runtime journeys themselves passed at the planned
  Windows built-process level.
- Smallest recommended correction/prerequisite: split the named staging test
  into smaller assertion helpers or subtests until its measured complexity is
  at most 15, then run the required UI tool setup and have the owners of the
  pre-existing boundary/baseline findings resolve or formally baseline them.
  Do not weaken runtime assertions or replace the built-process evidence.
- Dependencies and retest scope: after that correction, rerun `make lint`,
  `go test ./tests/integration/models -short -count=1 -timeout 45m -v`, the
  focused preflight suite, the race-qualified built subset, `go vet`, and both
  CLI contract gates. Emit a new clean-room report only from the resulting
  delivered head.
