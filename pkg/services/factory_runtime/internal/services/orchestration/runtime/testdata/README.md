# Cycle-003 missing-place witness

`cycle-003-missing-place.jsonl` is a seven-line semantic minimization of the
protected cycle-003 replay evidence: one v2 recording header, `RUN_REQUEST`,
`INITIAL_STRUCTURE_REQUEST`, the target `WORK_REQUEST`, its two target
relationship changes, and the post-restart `DISPATCH_REQUEST` for `plan`.
Unrelated Work, payloads, prompts, and later responses are intentionally not
copied.

The source recordings were checked read-only with PowerShell
`Get-FileHash -Algorithm SHA256`:

| Source | Bytes | Records | SHA-256 |
| --- | ---: | ---: | --- |
| `docs/temp/probes/factory-reliability-fr1-replay-identity-engineering-001/fixtures/0/preserved-before-restart.jsonl` | 23,606,575 | header + 5,801 events | `a4ce2fd1f587573224db5283f797b12fa549315cebb1e152aa3b6cac60873ee9` |
| `docs/temp/probes/factory-reliability-fr1-replay-identity-engineering-001/fixtures/1/live-final-sol-snapshot-20260911T0618Z.jsonl` | 20,304,421 | header + 5,043 events | `21d00963d5645a9799b90a22cb91046639a2afeb4160125f29e436a0ecb64dc5` |

The minimized fixture is 9,487 bytes, seven lines, and has SHA-256
`6a7dfe27a12ca781db8b55d311217ba27a6312a78499c5294cd4a6ab9dd4f4f7`.

Provenance for the retained source-0 facts:

- header line 1; `RUN_REQUEST` sequence 0 / line 2; and
  `INITIAL_STRUCTURE_REQUEST` sequence 1 / line 3;
- target `WORK_REQUEST` sequence 4776 / line 4778, including the target
  `idea/init` Work, its chain IDs, parent lineage tags, and the source
  `localai` Work;
- target `PARENT_CHILD` and `DEPENDS_ON` relationship changes, sequences
  4777–4778 / lines 4779–4780; and
- post-restart target `DISPATCH_REQUEST` sequence 4858 / line 4860, dispatch
  `e6629641-36f7-431f-b4bf-a2a3f6402eaa`, transition `plan`, and the unchanged
  Work-ID-only input shape.

The post-restart dispatch retains raw tick 1 while the Work request retains
tick 321. The production Recordings projection therefore applies the dispatch
before the later Work facts when it orders this out-of-order prefix, leaving
the projected input `PlaceID` empty while retaining the Work and dispatch
identities. `TestCycle003MissingPlaceProjectionPreservesLegacyWitness` drives
that prefix through `recordings/wire.NewProjectionService` and asserts the
detached projection.

The one-time pre-change Runtime constructor diagnostic was captured separately
before any production correction with
`go test ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run 'TestCycle003MissingPlace' -count=1 -v`, using the projected detached
state and the existing local Runtime constructor:

`restore Factory Runtime Work board: restore Work board: active dispatch "e6629641-36f7-431f-b4bf-a2a3f6402eaa" input for Work "batch-localai-project-cycle-092-v15-adapter-topology-characterization-20260910-localai-v3-model-adapter-topology-characterization-001" has no logical place`

The constructor returned no Factory (`factory=false`) with that error.
