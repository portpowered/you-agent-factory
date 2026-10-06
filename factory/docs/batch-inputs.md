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

* the batch file passed to `you submit batch` is the raw request object; it is
  not wrapped in `request`
* Project Leads and the Portfolio Supervisor submit that file through the CLI,
  check the accepted Work IDs, and return only a decision envelope from their
  workstation; their final response does not submit Work

* submit 3-5 `idea` work items per batch
* for non-Project (ideafy/supervisor) batches, submit one loopback `thoughts`
  work item and make it depend on the ideas through `DEPENDS_ON` relations
* Project Lead batches contain no loopback item; instead every idea and
  validation carries `"tags": {"project": "<project tag>"}` and each one wakes
  its lead when it reaches `complete` or `failed`
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

## Loopback proposals

A project-tagged thoughts loopback that finds gaps saves its raw corrective
batch in the main checkout at
`docs/temp/projects/<project>/proposals/<loopback-name>.json`, dry-runs it in
the bound Session and returns ACCEPTED naming the path. It submits no Project
children. Write/dry-run failures remain truthful FAILED results, retaining any
saved reference. Both outcomes pass through thoughts reporting states to their
original terminal state and one origin-preserving project-report. The existing
tag match wakes only the waiting owning lead; reports wait while it is busy.
The lead reviews/edits the draft against immutable authority and live ownership,
then submits with a verified receipt or records a reason and release event in
progress.md. Request/origin deduplication and the existing two-successor recovery
budget still apply. Untagged loopbacks retain their own dry-run/submission/receipt
or accepted hold and cannot wake a tagged peer Project.

Activate config and prompts together after independent validation and merge.
There is no report backfill. On rollback preserve drafts, Work and event history;
drain or operator-hold reporting Work before restoring config/prompts together.
Leads never perform Work controls to drain or acknowledge reports.

## Verification

When changing these factory-local docs or the checked-in example, run the
narrow verification path from the repository root:

```sh
go test ./pkg/services/workers/internal/prompting -run TestPromptRenderer_ResolvesCheckedInPlannerFactoryDocs -count=1
go test ./pkg/services/work/transports/cli/submit -run TestSubmitBatch_DryRunFactoryDocsBatchInputExample -count=1
```

The first command is the doc-path smoke check: it renders
`factory/docs/overview.md` and `factory/docs/batch-inputs.md` through the
checked-in factory directory using the same prompt `.Docs` resolution path workers
use at runtime. The second command proves `factory/docs/batch-input-example.json`
is accepted by the batch parser without syntax or contract-shape errors.
