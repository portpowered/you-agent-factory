# Root Factory Artifact Contract Inventory

This inventory classifies the root-level artifact assumptions that currently
matter to the targeted contract tests in `pkg/api`, `pkg/config`, `pkg/replay`,
`tests/adhoc`, and `tests/functional_test`.

## Classification Rules

- `checked_in`: the repository contract says the file or directory should exist
  in a checkout.
- `generated`: the artifact comes from a generation workflow and tests must
  classify it explicitly before depending on it.
- `obsolete`: the path belonged to an older starter or fixture surface and must
  not silently return as a root dependency.

## Inventory

| Path | Classification | Notes |
| --- | --- | --- |
| `factory/` | `checked_in` | Canonical checked-in repository starter root. |
| `factory/README.md` | `checked_in` | Documents the repository-local checked-in workflow. |
| `factory/factory.json` | `checked_in` | Canonical checked-in repository workflow config. |
| `factory/scripts/setup-workspace.py` | `checked_in` | Checked-in workspace setup helper used by the canonical repository workflow. |
| `factory/inputs/` | `checked_in` | Repository-local checked-in input surface for plan/task/idea/thought flows, materialized by tracked sentinels in each canonical inbox. |
| `factory/inputs/BATCH/default/` | `checked_in` | Checked-in canonical inbox for ordered or mixed-work-type `FACTORY_REQUEST_BATCH` submissions. |
| `factory/inputs/BATCH/default/.gitkeep` | `checked_in` | Tracked sentinel that keeps the canonical batch inbox present in clean checkouts. |
| `factory/inputs/idea/default/` | `checked_in` | Checked-in repository workflow idea inbox backed by a tracked `.gitkeep` sentinel. |
| `factory/inputs/idea/default/.gitkeep` | `checked_in` | Tracked sentinel that keeps the canonical idea inbox present in clean checkouts. |
| `factory/inputs/plan/default/` | `checked_in` | Checked-in repository workflow plan inbox backed by a tracked `.gitkeep` sentinel. |
| `factory/inputs/plan/default/.gitkeep` | `checked_in` | Tracked sentinel that keeps the canonical plan inbox present in clean checkouts. |
| `factory/inputs/task/default/` | `checked_in` | Checked-in repository workflow task inbox backed by a tracked `.gitkeep` sentinel. |
| `factory/inputs/task/default/.gitkeep` | `checked_in` | Tracked sentinel that keeps the canonical task inbox present in clean checkouts. |
| `factory/inputs/thoughts/default/` | `checked_in` | Checked-in repository workflow thought inbox backed by a tracked `.gitkeep` sentinel. |
| `factory/inputs/thoughts/default/.gitkeep` | `checked_in` | Tracked sentinel that keeps the canonical thought inbox present in clean checkouts. |
| `factory/logs/meta/asks.md` | `checked_in` | Canonical checked-in customer-ask backlog for the meta and cleaner workflow. |
| `factory/logs/meta/view.md` | `checked_in` | Checked-in meta world-state view consumed by the cleaner workflow. |
| `factory/logs/meta/progress.tsx` | `checked_in` | Checked-in meta progress surface consumed by the cleaner workflow. |
| `factory/logs/agent-fails.json` | `checked_in` | Event-stream sample for replay artifact conversion coverage. |
| `factory/logs/agent-fails.replay.json` | `checked_in` | Replay artifact sample paired with `agent-fails.json`. |
| `factory/logs/meta/asks.md` | `checked_in` | Checked-in meta ask surface for the repository-maintainer cleanup loop. |
| `factory/logs/meta/progress.tsx` | `checked_in` | Checked-in meta progress surface used by the repository-maintainer cleanup loop. |
| `factory/logs/meta/view.md` | `checked_in` | Checked-in meta world-view surface used by the repository-maintainer cleanup loop. |
| `tests/adhoc/factory-recording-04-11-02.json` | `checked_in` | Canonical replay fixture used by adhoc and replay package tests. |
| `tests/adhoc/factory/README.md` | `checked_in` | Checked-in adhoc fixture doc. |
| `tests/adhoc/factory/factory.json` | `checked_in` | Checked-in adhoc fixture config. |
| `ui/src/api/generated/openapi.ts` | `generated` | Generated TypeScript contract artifact. |
| `ui/src/components/dashboard/fixtures/failure-analysis-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
| `ui/src/components/dashboard/fixtures/graph-state-smoke-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
| `ui/src/components/dashboard/fixtures/resource-count-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
| `ui/src/components/dashboard/fixtures/runtime-details-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
| `factory/workers/processor/AGENTS.md` | `checked_in` | Checked-in canonical processor worker prompt. |
| `factory/workers/workspace-setup/AGENTS.md` | `checked_in` | Checked-in canonical workspace setup worker prompt. |
| `factory/workstations/cleaner/AGENTS.md` | `checked_in` | Checked-in canonical repository cleanup workstation prompt. |
| `factory/workstations/ideafy/AGENTS.md` | `checked_in` | Checked-in canonical ideation workstation prompt. |
| `factory/workstations/plan/AGENTS.md` | `checked_in` | Checked-in canonical planning workstation prompt. |
| `factory/workstations/process/AGENTS.md` | `checked_in` | Checked-in canonical execution workstation prompt. |
| `factory/workstations/review/AGENTS.md` | `checked_in` | Checked-in canonical review workstation prompt. |
| `factory/workers/executor/AGENTS.md` | `obsolete` | Legacy story-starter worker path. |
| `factory/workers/reviewer/AGENTS.md` | `obsolete` | Legacy story-starter worker path. |
| `factory/workstations/execute-story/AGENTS.md` | `obsolete` | Legacy story-starter workstation path. |
| `factory/workstations/review-story/AGENTS.md` | `obsolete` | Legacy story-starter workstation path. |
| `factory/inputs/story/default/example-story.md` | `obsolete` | Legacy story-starter seed file. |
| `factory/old/README.md` | `obsolete` | Legacy historical starter surface. |

## Guardrail

`pkg/testutil/artifact_contract_test.go` is the focused regression guard for
this inventory:

- every targeted root artifact dependency must be classified here
- every `checked_in` path must exist
- every `obsolete` path must stay absent
- the inventory doc table and the enforced classifications must stay in sync

That keeps root artifact drift explicit instead of letting missing files or
legacy starter assumptions re-enter the targeted package tests silently.

`make artifact-contract-closeout` is the integration closeout entrypoint for
this contract. It runs the inventory guard, reruns `make release-surface-smoke`,
and then reruns `go test ./pkg/api ./pkg/config ./pkg/replay ./tests/adhoc
./tests/functional_test` so starter-surface checks and self-contained package
tests fail together if a hidden legacy path dependency returns.
