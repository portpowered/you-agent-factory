# LocalAI inference recovery reconciliation ledger

This ledger records the boundary used to recover reviewed product intent from
closed PR #2487 onto current `main`. The retained source head is
`646d1b2e45be4fc43a224c370b677f8642f4be46`; the recovery starts from current
`main` at `e10e38843aff30c7871b732b284976ee13ab42f1`. PR #2487 was closed and
unmerged, so its evidence is historical and is not treated as current-head
proof.

The ledger is intentionally commit- and surface-oriented. A row marked
`replace` is the task-001 slice recovered with a current-main implementation;
`defer` remains owned by a later story; `drop` is outside this lane or is an
immutable/generated/baseline surface. No public Go, OpenAPI, CLI grammar, or
Factory graph contract is introduced here.

| #2487 commit | Hunk/surface | Disposition | Current path or owner | Reason |
| --- | --- | --- | --- | --- |
| `ddfda0e763` characterize defaults | Wire default/fallback characterization | replace | `pkg/services/models/wire/fallback_runtime.go`, `wire.go`, `omni_runtime_test.go` | Keep explicit protocol/backend fixtures while replacing the production input-echo default with a typed no-output failure. |
| `ddfda0e763` characterize defaults | HTTP generated-client binary round-trip test | drop | Existing HTTP contract tests | Public contract work is outside task 001 and no authored contract change is needed for this slice. |
| `3a980c9201ac` preserve OMNI media/host recovery | LocalAI protocol and scoped host hunks | defer | Task 003 and task 002 | Valid product intent, but protocol and host lifecycle are not silently absorbed into the fail-closed reconciliation slice. |
| `3d39e2798c6` launch built-in LocalAI inference | Effect fields, materialization, managed launcher, host wiring | defer | Task 002 | Backend-file propagation and one managed host are a separate shared-surface outcome. |
| `be0e194e224` bind ASR | ASR protocol, staging, and default binding | defer | Task 004 | ASR response/staging behavior remains out of task 001. |
| `82fa14ad340` bind EMBED | Embedding protocol/codec/runtime and fixture changes | defer | Task 005 | EMBED semantic output and finite-vector validation remain out of task 001. |
| `82fa14ad340` bind EMBED | `fallback_runtime.go` and generic/fallback selection | replace | `pkg/services/models/wire/fallback_runtime.go`, `wire.go` | This isolated hunk establishes the fail-closed production boundary; its adapter siblings remain deferred. |
| `9bf3fc3a129` reuse generic runtime cache | Cache transaction and preflight changes | defer | Task 002 | Cache reuse is needed for host recovery but is not required to classify the default runtime. |
| `7d937b688e16` pin embedding source | Catalog/source and embedding fixture changes | defer | Task 005 | Source identity and EMBED behavior remain a later vertical lane. |
| `aa2bdae713b` reject stale caches | Stale cache rejection | defer | Task 002 | The lifecycle/cache failure matrix is owned by the host story. |
| `3109c7a76d9` cache regression test shape | Test-only complexity reshaping | defer | Task 002 | It follows the deferred cache behavior and does not characterize task-001 behavior. |
| `58b36b94bde` remote inference completion | CLI remote transport selection | defer | Task 006 | Long-running remote inference is explicitly outside this story. |
| `e2cf0069d66` remote protocol coverage | CLI HTTP test relocation | defer | Task 006 | It proves the deferred remote transport behavior. |
| `fef41c443ec` host teardown through scopes | Runtime host shutdown and cleanup | defer | Tasks 002/006 | Host/lease/process teardown is a later lifecycle outcome. |
| `dd926dfdc68` scoped host cleanup | Host cleanup and broad echo-runtime deletion | replace | `pkg/services/models/wire/fallback_runtime.go`, focused wire tests | Preserve the fail-closed behavior, but retain `InputEchoInvocationRuntime` as an explicit controlled fixture so current direct TTS characterization remains available. Host cleanup remains deferred. |
| `646d1b2e45be` CI coverage reconciliation | CI tests and package/latency baselines | drop | Current-main quality/baseline owners | Baseline and CI inventory edits are explicitly excluded from this product story and are not copied onto the branch. |

Explicit exclusions: PR #2292 pull-progress/input-validation work, OmniVoice
D1 deletion, packaged TTS Factory changes, UI changes, new public contracts,
hand-edited generated outputs, and real LocalAI probing. The ledger does not
claim those deferred or dropped surfaces as acceptance evidence.
