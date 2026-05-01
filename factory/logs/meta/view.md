# meta view

## world state

- repository `HEAD` is `d844505` on `main` after `git pull` on April 30, 2026,
  and `origin/main` is at the same commit.
- the latest merged lane is still pull request `#18` (`ralph/api-clean`),
  merged on May 1, 2026; since then `main` has advanced with the direct
  `d844505` standards update.
- the canonical checked-in maintainer backlog remains
  `factory/logs/meta/asks.md`; no item in that file is marked urgent.
- the checked-in workflow inboxes on `HEAD` still contain only tracked
  `.gitkeep` sentinels, but this workspace has ignored local residue under the
  inbox surface:
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
- the repository is no longer in the dirty control-plane state described by the
  previous view:
  - `git status --short` is clean
  - the earlier local deletions and modified meta files are gone from this
    checkout
- there are already active cleanup/review lanes in GitHub:
  - open PR `#19` `systems-cleanup`
  - open PR `#17` `ci-cd`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
- the historical replay still points to a workflow-contract stability problem:
  - `process` completions in `factory/logs/agent-fails.replay.json`:
    `9 ACCEPTED`, `27 REJECTED`
  - rejected `process` outputs overwhelmingly return `<CONTINUE>`
  - `review` completions show `5 ACCEPTED`, `4 REJECTED`
- the replay churn is explained by the checked-in maintainer workflow itself:
  - `factory/workers/processor/AGENTS.md` configures stop token
    `<COMPLETE>` only
  - `factory/workstations/process/AGENTS.md` instructs the executor to emit
    `<CONTINUE>` until the whole PRD and PR state are done
  - `factory/factory.json` maps `process` rejection back to `task:init`, so
    partial progress is encoded as a retry loop
- the highest-risk system-deficit ask is still the throttle handling design,
  and the current implementation remains dispatcher-owned runtime policy rather
  than a config-authored guard:
  - `pkg/factory/subsystems/subsystem_dispatcher.go` stores provider/model pause
    state in `throttlePauses map[providerModelKey]providerModelPause`
  - pause state is mirrored into runtime snapshots and dashboard/world-view
    types for observability
  - throttle enforcement currently runs in two dispatcher-specific filter passes
    instead of one canonical guard path

## current blockers

1. the checked-in world view had drifted behind `HEAD` and still described a
   dirty checkout that no longer exists.
2. the replay evidence still shows repeated executor churn, but the root cause
   is the current process/review contract rather than an unexplained runtime
   regression.
3. the throttle ask is still broad at the backlog level, so the right next move
   is a narrow cleanup slice rather than the full guard redesign in one pass.

## theory of mind

- the control plane is stable enough again to dispatch new follow-up work; the
  earlier "do not queue more work from this checkout" constraint is obsolete.
- the repository now has two distinct cleanup opportunities:
  - workflow semantics: the executor loop treats successful partial progress as
    rejection, which inflates replay churn
  - system simplification: throttle pause handling duplicates enforcement logic
    inside the dispatcher
- the throttle architecture matches the customer complaint precisely: retry and
  failure normalization are part of the provider-failure contract, but throttle
  pause enforcement is a separate runtime policy with its own state and filters.
- the narrowest defensible response to the throttle ask is not the full
  event-log-backed guard redesign yet; it is reducing duplicate dispatcher
  throttle logic first so the current behavior has one simpler choke point.

## next best move

- update the checked-in meta world model and progress log now.
- queue one narrow cleanup idea that simplifies current throttle handling by
  removing duplicate dispatcher pause enforcement while preserving the existing
  runtime behavior and observability.
- leave the broader `INFERENCE_THROTTLE_GUARD` redesign for a later lane after
  the current throttle surface is smaller and easier to reason about.
- keep the executor/review `<CONTINUE>` churn as the next likely quality lane if
  the throttle simplification lands cleanly.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of April 30, 2026.
- the release/quality backlog already has active lanes in flight (`systems-cleanup`
  and `ci-cd`), while the throttle ask still lacks a queued narrow cleanup slice.
- the throttle simplification ask is the best next customer-facing lane because
  it targets real runtime behavior and removes duplicated policy logic.
