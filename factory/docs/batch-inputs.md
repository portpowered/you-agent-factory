# Batch Inputs

This directory records a human-readable example for ideafy/meta-planner batch
submission. These files are documentation, not live factory inputs.

Related factory-local docs and planner state:

* `factory/docs/overview.md` — planner loop, session inspection, and quality
  gates
* `docs/temp/progress.md`, `docs/temp/checklist.md`, and `docs/temp/meta.md` —
  live planner state files (local, not checked in)
* `factory/docs/batch-input-example.json` — checked-in canonical batch example

The source of truth for the live file-listener schema is:

```sh
you docs batch-inputs
```

The example JSON uses the canonical `FACTORY_REQUEST_BATCH` shape from
`you docs batch-inputs`:

* submit 3-5 `idea` work items per batch
* submit one loopback `thoughts` work item
* make the loopback depend on the ideas through `DEPENDS_ON` relations
* use `workTypeName`, not `workType`
* use `works[]`, not `items[]`
* prefer `you submit batch <path> --session <session_id>` for autonomous
  meta-planner submission when the factory is already running

Before submitting a real batch, dry-run against a live session:

```sh
you submit batch --dry-run factory/docs/batch-input-example.json --session <session_id>
```

Replace `<session_id>` with a live id from `you session list` (for example
`c803e7f7-1361-4ba6-bb2b-b5c9cfeb2754` on a long-running host).

## Verification

When changing these factory-local docs or the checked-in example, run the
narrow verification path from the repository root:

```sh
go test ./pkg/services/workers/prompting -run TestPromptRenderer_ResolvesCheckedInPlannerFactoryDocs -count=1
go test ./pkg/transports/cli/submit -run TestSubmitBatch_DryRunFactoryDocsBatchInputExample -count=1
```

The first command is the doc-path smoke check: it renders
`factory/docs/overview.md` and `factory/docs/batch-inputs.md` through the
checked-in factory directory using the same prompt `.Docs` resolution path workers
use at runtime. The second command proves `factory/docs/batch-input-example.json`
is accepted by the batch parser without syntax or contract-shape errors.
