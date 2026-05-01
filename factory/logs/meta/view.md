# meta view

## world state

- repository head is `1542835` on `main` after `git pull --ff-only` on
  April 30, 2026.
- the previously open inbox-hygiene lane is already landed on `main`:
  - pull request `#15` merged at `2026-05-01T01:22:03Z`
  - that corresponds to April 30, 2026 in `America/Los_Angeles`
  - `main` now carries the prune-solved-local-workflow-input-residue lane
- the canonical checked-in customer-ask backlog is still centralized:
  - `factory/logs/meta/asks.md` is the canonical checked-in backlog
  - `factory/meta/asks.md` remains a redirect-only compatibility stub
  - the live ask categories still include `release plans`,
    `system deficits`, and `quality`
  - no ask is marked urgent
- the live checked-in workflow inboxes are currently clean:
  - `factory/inputs/idea/default/` contains only `.gitkeep`
  - `factory/inputs/task/default/` contains only `.gitkeep`
  - `factory/inputs/BATCH/default/` contains only `.gitkeep`
  - the previously suspected `factory/inputs/plan/default/dedupe-root-factory-artifact-contract-entries.md`
    file is ignored local residue in this workspace, not a tracked file on
    `HEAD`
- the checked-in root artifact contract docs still point at the right control
  plane:
  - `docs/development/root-factory-artifact-contract-inventory.md` classifies
    `factory/logs/meta/progress.txt` as the canonical checked-in progress
    surface
  - `docs/processes/factory-workstation-relevant-files.md` and the checked-in
    meta instructions also treat `factory/logs/meta/asks.md` as the only live
    customer backlog
- the remaining control-plane drift is now concentrated in the progress
  surface:
  - `factory/logs/meta/progress.tsx` is still the tracked file on `HEAD`
  - `factory/logs/meta/progress.txt` was missing before this pass even though
    the contract docs and maintainer instructions expect it

## current blockers

1. the checked-in meta world-state surfaces had drifted behind `main` and were
   still describing solved inbox residue as if it were repository truth.
2. the canonical checked-in progress surface was missing even though the public
   workflow contract and maintainer prompts expect `factory/logs/meta/progress.txt`.
3. the tracked legacy `factory/logs/meta/progress.tsx` surface still conflicts
   with the documented `progress.txt` contract and can mislead future
   maintainers about which path is canonical.

## theory of mind

- the repository is stable enough to avoid a broad stability audit.
- the highest-value work is still control-plane honesty and cleanliness, not a
  broad feature ask from the non-urgent backlog.
- the checked-in ask backlog and live workflow inboxes are now cleaner than the
  prior world model claimed.
- the immediate risk has shifted from inbox residue to contract drift around
  the meta progress surface.
- local ignored files under `factory/inputs/**` and `factory/logs/meta/*` are
  not repository truth and should be verified with `git ls-files` before they
  influence the checked-in world model.
- the right customer rule remains:
  - `factory/logs/meta/asks.md` is the only live checked-in customer-ask
    backlog
  - the current asks are backlog inputs, not approved in-flight work
  - stability and world-model accuracy stay ahead of speculative product work
    unless an ask is marked urgent

## next best move

- do not start the release, CI/CD, throttle-guard, or website-quality asks yet.
- keep the checked-in meta surfaces current with `main`.
- restore `factory/logs/meta/progress.txt` as the canonical checked-in progress
  surface for this workflow.
- dispatch one narrow cleanup idea that retires the legacy tracked
  `factory/logs/meta/progress.tsx` path and reconciles any prompt or guard
  references that still imply the wrong control-plane file.
- after that progress-surface lane lands, reassess whether the next move should
  be another narrow control-plane pass or the highest-value non-urgent customer
  ask.

## customer asks

- `factory/logs/meta/asks.md` currently carries active asks under `release
  plans`, `system deficits`, and `quality`.
- no explicit urgency marker or top-ranked ask is recorded in the checked-in
  backlog.
