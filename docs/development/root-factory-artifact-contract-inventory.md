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
| `factory/inputs/` | `checked_in` | Repository-local checked-in input surface for plan/task/idea/thought flows. |
| `factory/logs/agent-fails.json` | `checked_in` | Event-stream sample for replay artifact conversion coverage. |
| `factory/logs/agent-fails.replay.json` | `checked_in` | Replay artifact sample paired with `agent-fails.json`. |
| `tests/adhoc/factory-recording-04-11-02.json` | `checked_in` | Canonical replay fixture used by adhoc and replay package tests. |
| `tests/adhoc/factory/README.md` | `checked_in` | Checked-in adhoc fixture doc. |
| `tests/adhoc/factory/factory.json` | `checked_in` | Checked-in adhoc fixture config. |
| `ui/src/api/generated/openapi.ts` | `generated` | Generated TypeScript contract artifact. |
| `ui/src/components/dashboard/fixtures/failure-analysis-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
| `ui/src/components/dashboard/fixtures/graph-state-smoke-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
| `ui/src/components/dashboard/fixtures/resource-count-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
| `ui/src/components/dashboard/fixtures/runtime-details-events.ts` | `checked_in` | Checked-in dashboard replay fixture. |
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

That keeps root artifact drift explicit instead of letting missing files or
legacy starter assumptions re-enter the targeted package tests silently.
