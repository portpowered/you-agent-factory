# `@you/goal` Active PR Refresh

Last updated: 2026-06-26T09:06:00Z

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

## Completed PRs Removed From The Active Batch

The closed `@you/goal` PR inventory currently contains only merged heads. Those
PRs are removed from the active batch because they already landed on `main` and
do not require new factory execution for this refresh cycle.

| PR | Branch | Completed state | Why it is no longer active |
| --- | --- | --- | --- |
| [#836](https://github.com/portpowered/you-agent-factory/pull/836) | `you-goal-p01-register-packaged-factory` | `MERGED` at `2026-06-20T06:05:09Z` | Landed on `main`; removed from the active batch. |
| [#840](https://github.com/portpowered/you-agent-factory/pull/840) | `you-goal-p02-materialize-editable-layout` | `MERGED` at `2026-06-20T06:54:33Z` | Landed on `main`; removed from the active batch. |
| [#842](https://github.com/portpowered/you-agent-factory/pull/842) | `you-goal-p03-add-minimal-goal-topology` | `MERGED` at `2026-06-20T08:07:43Z` | Landed on `main`; removed from the active batch. |
| [#848](https://github.com/portpowered/you-agent-factory/pull/848) | `you-goal-p04-add-split-prompts` | `MERGED` at `2026-06-21T15:19:09Z` | One of the recent merges that shifted the active batch; no new execution needed. |
| [#847](https://github.com/portpowered/you-agent-factory/pull/847) | `you-goal-p05-configure-primary-result` | `MERGED` at `2026-06-20T10:47:59Z` | Landed on `main`; removed from the active batch. |
| [#851](https://github.com/portpowered/you-agent-factory/pull/851) | `you-goal-p06-prove-cli-api-invocation-parity` | `MERGED` at `2026-06-20T12:43:17Z` | Landed on `main`; removed from the active batch. |
| [#849](https://github.com/portpowered/you-agent-factory/pull/849) | `you-goal-p07-define-work-propagation-config` | `MERGED` at `2026-06-20T10:57:20Z` | Landed on `main`; removed from the active batch. |
| [#852](https://github.com/portpowered/you-agent-factory/pull/852) | `you-goal-p08-implement-work-propagation-routing` | `MERGED` at `2026-06-20T12:36:38Z` | Landed on `main`; removed from the active batch. |
| [#846](https://github.com/portpowered/you-agent-factory/pull/846) | `you-goal-p09-define-structured-decision-envelope` | `MERGED` at `2026-06-20T11:47:32Z` | Landed on `main`; removed from the active batch. |
| [#853](https://github.com/portpowered/you-agent-factory/pull/853) | `you-goal-p11-api-contract-audit` | `MERGED` at `2026-06-20T13:07:05Z` | Landed on `main`; removed from the active batch. |
| [#858](https://github.com/portpowered/you-agent-factory/pull/858) | `you-goal-p12-add-internal-session-stream-model` | `MERGED` at `2026-06-21T15:40:09Z` | One of the recent merges that shifted the active batch; no new execution needed. |
| [#841](https://github.com/portpowered/you-agent-factory/pull/841) | `you-goal-p19-land-pause-resume-signal-reliability` | `MERGED` at `2026-06-21T15:20:50Z` | One of the recent merges that shifted the active batch; no new execution needed. |
| [#845](https://github.com/portpowered/you-agent-factory/pull/845) | `you-goal-p23-add-packaged-goal-docs` | `MERGED` at `2026-06-20T09:16:15Z` | Landed on `main`; removed from the active batch. |

## Follow-Up Tasks That Are Not Reopened PR Lanes

The remaining follow-up references `P20` and `P26` are not part of the
completed-PR inventory above. No current `gh pr list --search 'you-goal-p20 OR
you-goal-p26 in:title,head' --state all` result exists, so this refresh treats
them as still-open follow-up planning or execution tasks rather than merged PRs
or reopened active-branch lanes.

## Remaining Sections

The overlap audit and concrete merge order are tracked by later P27 stories and
are intentionally left for follow-up iterations.
