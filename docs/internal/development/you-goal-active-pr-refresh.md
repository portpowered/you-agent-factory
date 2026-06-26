# `@you/goal` Active PR Refresh

Last updated: 2026-06-26T09:01:51Z

This artifact tracks the current maintainer-facing refresh for the remaining
open `@you/goal` PR batch after the recent merges.

## Current Open PR Set

The raw source query `gh pr list --search 'you-goal in:title,head' --state
open` currently returns four PRs: `#854`, `#856`, `#859`, and this refresh PR
`#874`.

This artifact tracks the remaining maintainer work batch rather than the
refresh artifact PR itself, so it intentionally excludes `#874` from the table
below. The exclusion rule is: keep the raw query result as evidence, but only
list the still-open `@you/goal` implementation PRs whose blockers or merge
ordering this refresh is meant to coordinate.

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

## Overlap Audit And Contract Boundaries

This overlap audit uses the current PR diffs plus the latest PR conversation
comments as the maintainer evidence set. The goal is to show where the remaining
PRs still touch the same risk surfaces and whether the next action is a rebase,
content narrowing, or a prerequisite merge.

| Surface | Open PRs touching it | Evidence | Why it overlaps | Smallest next owner action |
| --- | --- | --- | --- | --- |
| Packaged-goal topology or config | `#854`, `#856` | Both PRs change `pkg/config/layout.go`. | `#854` rewires built-in `@you/goal` review routing while `#856` depends on the same built-in layout for named-goal invocation and persisted materialization behavior. This is a real content-overlap surface, not just shared verification. | `#854`: `rebase` on current `main` before rerunning checks. `#856`: `narrow` or add an upgrade path so its claim matches the persisted-materialization behavior it actually proves. |
| `docs/reference/packaged-goal.md` | `#856` only | `gh pr diff 856 --name-only` includes `docs/reference/packaged-goal.md`; `#854` and `#859` do not. | No active cross-PR conflict today, but this page is still a customer-contract surface because `#856` broadens the headless-use claim. | `#856`: update docs only after the existing-materialization path is either fixed or the claim is narrowed to fresh materialization. |
| Invocation behavior | `#854`, `#856` | `#854` changes packaged-goal routing and `docs/internal/processes/invocation-relevant-files.md`; `#856` changes named-goal CLI smoke coverage, packaged-goal docs, and the same invocation-process map. | Both PRs affect how `@you/goal` reaches terminal invocation output, but through different layers: `#854` changes packaged-factory routing and summary shaping, while `#856` changes the customer-facing headless invocation claim and verification. | `#854`: `rebase` only. `#856`: `narrow` until it proves the reused materialized factory path or ships an upgrade path. |
| Dispatch lifecycle or control surfaces | `#859` only | `gh pr diff 859 --name-only` shows `pkg/factorysessionexecution/*`, `pkg/service/runtime_sessions.go`, lifecycle tests, and interrupt API surface files. | No current overlap with `#854` or `#856`, but this PR still sits on a shared session-control surface that may depend on later follow-up work such as `P20`/`P26` verification expectations. | `#859`: `rebase` and fix the current `Build, Lint, and API` failure on the rebased head before merge. |
| Authored OpenAPI fragments | `#859` only | `gh pr diff 859 --name-only` includes `api/openapi-main.yaml` and new API component fragments. | This is an isolated public-contract lane among the remaining open PRs. No active overlap with the packaged-goal PRs was found in the current diff set. | `#859`: keep the interrupt surface isolated and rerun the API/build lane after rebase. |
| Generated artifacts | `#859` only | `gh pr diff 859 --name-only` includes `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, and `ui/src/api/generated/openapi.ts`. | Generated churn is isolated to the interrupt-dispatch PR, so maintainers should avoid mixing it with packaged-goal rebases unless required by a later base-branch change. | `#859`: regenerate only from the final rebased authored contract and verify the generated diff stays isolated to this lane. |

### Current Boundary Checks

- `Workstation.workPropagation` remains the structured object contract, not a
  flattened enum field. The current authored schema is
  `api/components/schemas/data-models/Workstation.yaml` with companion enum
  `api/components/schemas/data-models/WorkPropagationMode.yaml`, and the only
  supported mode values remain `OUTPUT_AS_PAYLOAD` and `PRESERVE_INPUT`.
- Internal response streams remain internal-only. The current audit sources
  `docs/internal/development/plans/you-goal/api-contract-audit.md` and
  `docs/internal/processes/api-relevant-files.md` still require
  `SessionResponseStream` and `SessionResponseStreamEvent` to stay out of
  `FactoryEvent`, authored OpenAPI fragments, and generated client contracts.
- None of the remaining open PR diffs add `SessionResponseStream*` to
  `api/components/`, `api/openapi-main.yaml`, `api/openapi.yaml`, or generated
  client outputs, so the internal-stream boundary remains intact in the current
  active batch.

## Concrete Merge Order And Final Verification Prerequisites

The lowest-risk landing sequence is `#854` -> `#859` -> `#856`.

This order keeps the packaged-goal topology and invocation-contract overlap on a
single lane first, lands the isolated interrupt-dispatch contract lane second,
and leaves the only PR with still-unresolved blocking conversation feedback
last.

| Order | PR | Why this position is lowest-risk | Current blocker to clear first | Smallest next owner action |
| --- | --- | --- | --- | --- |
| 1 | [#854](https://github.com/portpowered/you-agent-factory/pull/854) | `#854` and `#856` both change the built-in `@you/goal` layout plus invocation-adjacent process docs, but `#854` no longer has unresolved blocking conversation feedback. Landing it first removes one shared `pkg/config/layout.go` conflict source before the more correctness-sensitive `#856` lane is reworked. | GitHub still reports `DIRTY`, and the latest recorded failing required lane on the reviewed head was `Backend Verification` before later merges changed mergeability. | Rebase `#854` on current `main`, resolve `pkg/config/layout.go` and any nearby invocation-doc drift, rerun required checks, then merge if the rebased head stays equivalent to the already-addressed review scope. |
| 2 | [#859](https://github.com/portpowered/you-agent-factory/pull/859) | `#859` is isolated from the packaged-goal layout/docs lanes. Its overlap is on durable session control, authored OpenAPI, and generated artifacts, so it should land once the packaged-goal conflict lane above is out of the way and without waiting on `#856`. | GitHub still reports `DIRTY`, and the latest required `Build, Lint, and API` run on the reviewed head failed before the rebased fix reply. | Rebase `#859` on the post-`#854` `main`, regenerate API outputs from the rebased authored contract if needed, rerun the full required quality gate, and merge once the rebased head reproduces the reply-commented fix. |
| 3 | [#856](https://github.com/portpowered/you-agent-factory/pull/856) | `#856` is the only remaining lane with unresolved blocking conversation feedback, and it overlaps `#854` on built-in `@you/goal` layout and invocation-facing docs. Leaving it last avoids forcing later PRs to build on top of a branch whose product claim is still too broad for the proved behavior. | The latest blocking comment says previously materialized `@you/goal` installs still fail with `template: prompt:1:39: executing "prompt" at <.WorkID>` and `INVOCATION_PRIMARY_RESULT_UNRESOLVED`, and the PR is still merge-conflicting. | Rebase after `#854` lands, then either add an upgrade path for existing materialized built-ins or narrow the docs/tests/claim to fresh materialization only; reply in PR conversation comments with the exact path chosen and the focused verification that proves it. |

### Why `P20` And `P26` Do Not Block This Order

- No active PR currently exists for `P20` or `P26`, so neither item can be a
  prerequisite merge target in the present batch.
- For `#854`, there is no current evidence that `P20` or `P26` changes are
  needed before final verification; the lane is blocked by ordinary rebase and
  rerun work only.
- For `#859`, the only plausible dependency is verification interpretation on
  shared session-lifecycle surfaces. Treat `P20`/`P26` as post-merge follow-up
  context for lifecycle expectations, not as hidden blockers for rebasing and
  merging the current interrupt-dispatch head.
- For `#856`, `P20` and `P26` are unrelated to the documented blocker. The
  unresolved issue is specifically the persisted built-in `@you/goal` reuse
  path, so that lane must be fixed or narrowed on its own merits.

### Final Verification Prerequisites By PR

- `#854`: rebase on current `main`, rerun required checks, and confirm the
  rebased diff still preserves the structured-review and plain-review routing
  behavior already mapped in PR conversation replies.
- `#859`: rebase on current `main`, regenerate any affected OpenAPI outputs,
  rerun the required CI lanes, and confirm the interrupt replay semantics still
  match the reply-commented reconnect expectations after rebase.
- `#856`: do not treat green-on-fresh-install evidence as sufficient. Final
  verification must cover either the upgrade path for an already materialized
  built-in `@you/goal` factory or a narrowed customer claim that explicitly
  excludes that persisted path.
