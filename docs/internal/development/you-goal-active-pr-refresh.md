# `@you/goal` Active PR Refresh

Last updated: 2026-06-26T08:51:35Z

This artifact tracks the current maintainer-facing refresh for the remaining
open `@you/goal` PR batch after the recent merges.

## Current Open PR Set

The active batch is limited to the still-open `@you/goal` heads returned by
`gh pr list --search 'you-goal in:title,head' --state open`: `#854`, `#856`,
and `#859`. No other open PR head currently matches the remaining
`@you/goal` lane, so this refresh does not re-add completed work to the active
set.

| PR | Branch | Current merge state | Latest blocker | Recommended next action |
| --- | --- | --- | --- | --- |
| [#854](https://github.com/portpowered/you-agent-factory/pull/854) | `you-goal-p10-wire-decision-routing` | `DIRTY` | The latest blocking conversation comments were explicitly addressed on 2026-06-20T14:05:00Z and 2026-06-20T14:30:00Z UTC, but the current GitHub head is no longer merge-clean after later merges. The head must be rebased and required checks rerun on the rebased diff. | `rebase` |
| [#856](https://github.com/portpowered/you-agent-factory/pull/856) | `you-goal-p18-confirm-batch-headless-behavior` | `DIRTY` | The latest unresolved conversation comment says existing materialized `@you/goal` installs still fail with `template: prompt:1:39: executing "prompt" at <.WorkID>` and `INVOCATION_PRIMARY_RESULT_UNRESOLVED`, and also notes the PR is merge-conflicting. The current claim is broader than the proven fresh-materialization path. | `narrow` |
| [#859](https://github.com/portpowered/you-agent-factory/pull/859) | `you-goal-p21-interrupt-active-dispatch` | `DIRTY` | The latest blocking review was mapped to a fix reply on 2026-06-20 UTC, but the open PR still shows a dirty merge state and a failing `Build, Lint, and API` run on the current GitHub head. It needs a rebase plus a fresh quality-gate rerun from the rebased head. | `rebase` |

## Evidence Snapshot

- Open active set source: `gh pr list --search 'you-goal in:title,head' --state open`
- PR `#854`: `gh pr view 854 --json ...` plus PR conversation comments
- PR `#856`: `gh pr view 856 --json ...` plus PR conversation comments
- PR `#859`: `gh pr view 859 --json ...` plus PR conversation comments

## Remaining Sections

The completed-PR inventory, overlap audit, and concrete merge order are tracked
by later P27 stories and are intentionally left for follow-up iterations.
