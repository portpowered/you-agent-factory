# meta view

## world state

- repository `HEAD` is `b47e277` on `main` after `git pull --ff-only` on
  April 30, 2026, and `origin/main` is at the same commit.
- the latest merged lane since the prior meta refresh is pull request `#18`
  (`ralph/api-clean`), which landed the API schema cleanup and regenerated
  contract surfaces on `main`.
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent.
- the checked-in workflow inboxes on `HEAD` still contain only tracked
  `.gitkeep` sentinels, but this workspace also has local non-canonical residue:
  - `factory/inputs/idea/default/api-clean.md`
  - `factory/inputs/idea/default/ci-cd.md`
  - `factory/inputs/plan/default/retire-legacy-meta-progress-surface.md`
- this checkout is currently mid-cleanup in the meta control plane:
  - `factory/logs/meta/progress.tsx` is deleted locally
  - `factory/meta/asks.md` is deleted locally
  - `factory/workstations/cleaner/AGENTS.md` is modified locally
  - several artifact-contract and functional-test files are also in local flux
- the upstream artifact-contract mismatch from earlier refreshes is no longer
  the primary blocker:
  - `internal/testpath/artifact_contract.go` now classifies
    `factory/logs/meta/progress.txt`
  - `git ls-files` still shows both `factory/logs/meta/progress.tsx` and
    `factory/logs/meta/progress.txt` on `HEAD`, so the duplicate progress
    surface remains a repository concern until the local deletion lands
- the historical failure replay still shows a stability problem in the execution
  loop rather than a new control-plane mismatch:
  - `process` completions in `factory/logs/agent-fails.replay.json`:
    `9 ACCEPTED`, `27 REJECTED`
  - rejected `process` outputs are overwhelmingly `<CONTINUE>`
  - `review` completions show `5 ACCEPTED`, `4 REJECTED`
- the customer backlog still contains broad asks in three clusters:
  `release plans`, `system deficits`, and `quality`
- the highest-risk system-deficit ask is still the throttle handling design:
  current runtime behavior uses dispatcher-owned provider/model pause state and a
  factory option (`WithProviderThrottlePauseDuration`) rather than a
  config-authored top-level guard

## current blockers

1. the checked-in world view had drifted behind `HEAD` and no longer described
   the current repository state after `#18` merged.
2. this workspace has in-flight local cleanup on the meta control plane, so the
   repo is not in a clean state for dispatching more follow-up work from the
   same surfaces.
3. the replay evidence still points to repeated execution/review loop churn,
   which makes a new broad customer ask less urgent than keeping the maintainer
   world model accurate.

## theory of mind

- the repository has moved past the earlier `progress.txt` classification bug,
  but it has not yet cleanly retired the duplicate meta progress and legacy ask
  surfaces in this checkout.
- the local deletions of `factory/logs/meta/progress.tsx` and
  `factory/meta/asks.md` indicate an active attempt to simplify the control
  plane; until that lands or is reconciled, the correct meta action is to
  describe the state precisely rather than pile on a second cleanup lane.
- the replay fixture suggests the core runtime still suffers more from repeated
  `<CONTINUE>` loops than from missing backlog decomposition.
- the throttle-pause architecture in the customer backlog is a real design
  target, but it is still a broad request that should be queued only after the
  current control-plane cleanup is settled.

## next best move

- update the checked-in meta world model and progress log now.
- do not start a non-urgent customer ask yet.
- do not queue a second cleanup idea from this checkout while the existing local
  meta-surface cleanup is still uncommitted.
- reassess the backlog after the current local control-plane changes either land
  or are explicitly abandoned, then choose between:
  - a narrow throttle-guard design idea
  - a quality/stability lane focused on repeated `process -> <CONTINUE>` churn

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of April 30, 2026.
- the throttle-guard simplification ask remains the most architecturally
  meaningful future lane once the control plane is clean.
