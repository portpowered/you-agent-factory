# meta view

## world state

- repository head is `5640325` on `main` after `git pull` reported `Already up to date.` on April 30, 2026.
- the latest landed root-factory stabilization sequence is now clearly on `main`:
  - `625157a` inventoried artifact path assumptions
  - `77de019` restored the canonical checked-in starter surface
  - `e5ddd22` closed the stabilized artifact contract
  - merge commit `5640325` landed that lane as pull request `#2`
- there is still no live customer work:
  - `factory/logs/meta/ask.md` is absent
  - `factory/logs/meta/asks.md` says there are no customer asks
- the local worktree is still dirty in one cleanup lane:
  - modified files exist in `pkg/api`, `pkg/config`, `pkg/interfaces`, `pkg/petri`, `factory/workstations/*/AGENTS.md`, and `docs/development/development.md`
  - untracked `factory/logs/meta/` exists in this workspace
- the broad handwritten-source contract guards now share the same hidden-dir behavior in the dirty workspace:
  - `pkg/api/legacy_model_guard_test.go`
  - `pkg/petri/transition_contract_guard_test.go`
  - `pkg/interfaces/runtime_lookup_contract_guard_test.go`
  - `pkg/interfaces/world_view_contract_guard_test.go`
  - `pkg/config/exhaustion_rule_contract_guard_test.go`
  - all now skip hidden metadata directories such as `.claude`, `.git`, and nested `.worktrees`
- the focused guard bundle is green in the current workspace after the ownership cleanup:
  - `go test ./pkg/api ./pkg/petri ./pkg/interfaces ./pkg/config -count=1`
- the skip-policy ownership cleanup now exists in the workspace:
  - `pkg/testutil/contractguard/skip.go` owns the shared hidden-dir rule
  - `pkg/api`, `pkg/petri`, `pkg/interfaces`, and `pkg/config` now keep only thin local wrappers that list their package-specific generated-dir exceptions
  - the helper lives under `pkg/testutil/contractguard` instead of `pkg/testutil` to avoid a `pkg/interfaces` import cycle
- the live instruction drift is corrected in the workspace but not yet landed:
  - `factory/workstations/cleaner/AGENTS.md` now points at checked-in idea files instead of a missing standard file
  - `factory/workstations/ideafy/AGENTS.md` now points at the current checked-in idea shape
  - `docs/development/development.md` now describes the repository-root layout instead of stale `libraries/agent-factory` paths
- the backlog still contains the right historical cleanup family for this area:
  - `factory/inputs/idea/default/standardize-contract-guard-skip-policy.md`
  - `factory/inputs/idea/default/reduce-contract-guard-skip-helper-ownership-after-green-conformance-lane.md`
- this run dispatched and completed one isolated worker lane for skip-helper ownership reduction.

## current blockers

1. the instruction and guard fixes are still workspace-only edits, so the corrected world model is not durable on `main` yet.
2. archival reports still mention `libraries/agent-factory`, but that wording is historical noise rather than a live operator contract.
3. the new shared skip helper is implemented and tested, but it still needs the dirty lane to be reviewed and landed.

## theory of mind

- the artifact/starter contract is stable on `main`; the active instability is cleanup hygiene and ownership discipline.
- the hidden-dir problem has changed shape again:
  - policy coverage exists where it matters
  - duplicated implementation has now been reduced in the workspace
  - the remaining gap is landing discipline, not helper design
- the right live layout model is:
  - this repository is rooted at the current checkout
  - `pkg/`, `tests/functional_test/`, `factory/`, and `docs/` are the live surfaces
  - `libraries/agent-factory` should be treated as stale historical wording, not the current path contract
  - the checked-in broad-walker inventory in `docs/development/contract-guard-walker-inventory.md` is the live cleanup reference for scan roots, hidden-dir policy, and generated-output exclusions
- the right cleanup rule now is:
  - do not reopen another broad audit
  - do not reopen the ownership design question unless the landing review finds a concrete issue
  - keep package-specific generated-dir exceptions explicit instead of hiding them behind opaque conditionals
- the right customer rule now is:
  - there is no ask to handle
  - stability and theory-of-mind accuracy are still higher value than speculative product work

## next best move

- keep customer handling deferred until there is an actual ask or an urgent request appears.
- land the current guard/doc lane cleanly.
- keep the new `pkg/testutil/contractguard` helper as the single shared owner for hidden-dir skipping unless review finds a better equally narrow seam.
- reuse the existing cleanup backlog shape instead of opening another overlapping audit lane.

## customer asks

- `factory/logs/meta/asks.md` currently says there are no customer asks.
- no urgency marker exists anywhere in the meta ask surface.
