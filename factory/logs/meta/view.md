# meta view

## world state

- repository head is `5d75096` on `main` after `git pull --ff-only` completed
  on April 30, 2026.
- the latest landed cleanup sequence is merge commit `5d75096` from pull
  request `#5`, which closed the remaining contract-guard hygiene and
  instruction-path alignment lane:
  - `fdb3e54` inventoried the remaining broad contract-guard walkers
  - `b3b8c39` hardened the remaining walkers and settled the skip-policy shape
  - `5d424c9` aligned active ideation and planning prompts with the real
    checkout layout
  - `183ffe9` updated the checked-in cleanup guidance to match the repository
    world model
- the prior major stabilization sequence remains the root starter and artifact
  contract lane:
  - `625157a` inventoried artifact path assumptions
  - `77de019` restored the canonical checked-in starter surface
  - `e5ddd22` added the closeout verification
  - merge commit `5640325` landed that sequence as pull request `#2`
- the canonical checked-in customer-ask backlog is active and centralized:
  - the canonical checked-in customer-ask backlog is
    `factory/logs/meta/asks.md`
  - the live ask categories currently include `release plans`,
    `system deficits`, and `quality`
  - no checked-in `plan`, `task`, or `thoughts` work items exist beyond the
    tracked `.gitkeep` sentinels
- the checked-in backlog is still crowded with already-explored contract-guard
  ideas:
  - `factory/inputs/idea/default/standardize-contract-guard-skip-policy.md`
  - `factory/inputs/idea/default/centralize-contract-guard-skip-policy-owner.md`
  - `factory/inputs/idea/default/reduce-contract-guard-skip-helper-ownership-after-green-conformance-lane.md`
  - adjacent idea files in the same directory still describe the same
    contract-guard and path-drift lane from slightly different angles
- the active repeated cleanup gap has moved into test scaffolding rather than
  production behavior:
  - `rg` over `*_test.go` still finds runtime-lookup doubles spread across
    `pkg/workers`, `pkg/service`, `pkg/factory`, and
    `tests/functional_test/logical_move_test.go`
  - full `RuntimeConfigLookup` stubs are still duplicated in
    `pkg/workers/agent_test.go` and `tests/functional_test/logical_move_test.go`
  - workstation-only or worker-plus-workstation map-backed doubles are still
    repeated in:
    - `pkg/factory/event_history_test.go`
    - `pkg/factory/projections/topology_projection_test.go`
    - `pkg/factory/runtime/factory_test.go`
    - `pkg/factory/scheduler/work_queue_test.go`
    - `pkg/factory/state/net_test.go`
    - `pkg/factory/subsystems/circuitbreaker_test.go`
    - `pkg/factory/subsystems/dispatcher_test.go`
    - `pkg/factory/subsystems/history_transitioner_pipeline_test.go`
    - `pkg/factory/subsystems/subsystem_transitioner_test.go`
    - `pkg/service/factory_test.go`
  - `pkg/replay/EmbeddedRuntimeConfig` is the real production implementation
    and should not be folded into test-only helpers
- the former duplicate ask path `factory/meta/asks.md` has been retired from
  the checked-in artifact contract, so the canonical ownership rule now has one
  checked-in backlog surface to protect.

## current blockers

1. the checked-in meta surfaces now need ongoing accuracy checks so the
   canonical ask summary in `factory/logs/meta/view.md` keeps matching
   `factory/logs/meta/asks.md`.
2. the repo still lacks one shared test-owned runtime lookup fixture seam, so
   small test changes continue to pay for duplicate `FactoryDir`,
   `RuntimeBaseDir`, `Worker`, and `Workstation` scaffolding.
3. the checked-in cleanup backlog is crowded with overlapping contract-guard
   ideas, which increases the risk of reopening a solved audit instead of
   reducing the next real source of duplication.

## theory of mind

- the repository is stable enough to avoid another broad stability audit.
- the artifact/starter contract and the follow-on contract-guard hygiene lane
  are both landed on `main`; the highest-value remaining work is cleanliness
  and ownership reduction, not another behavioral stabilization sweep.
- the right live layout model is:
  - this repository is rooted at the current checkout
  - `pkg/`, `tests/functional_test/`, `factory/`, and `docs/` are the live
    surfaces
  - `libraries/agent-factory` should be treated as stale historical wording,
    not the current path contract
  - the checked-in broad-walker inventory in
    `docs/development/contract-guard-walker-inventory.md` is the live cleanup
    reference for scan roots, hidden-dir policy, and generated-output
    exclusions
- the contract-guard hidden-dir problem is no longer the highest-value next
  idea because:
  - the docs/path drift and walker inventory landed in pull request `#5`
  - the checked-in idea backlog already covers the helper-ownership lane
  - reopening that topic again would duplicate intent rather than simplify code
- the next useful cleanliness move is to reduce repeated runtime-lookup test
  doubles while preserving the current production contract layering:
  - `RuntimeWorkstationLookup`
  - `RuntimeDefinitionLookup`
  - `RuntimeConfigLookup`
- the right boundary is test-only:
  - consolidate package-local test stubs under `pkg/testutil`
  - leave real runtime implementations such as `pkg/config.LoadedFactoryConfig`
    and `pkg/replay.EmbeddedRuntimeConfig` alone
- the right customer rule now is:
  - `factory/logs/meta/asks.md` is the canonical checked-in customer-ask
    backlog for the meta workflow
  - the current checked-in asks are backlog inputs, not approved in-flight
    product work
  - stability and theory-of-mind accuracy are still higher value than
    speculative product work

## next best move

- do not open another contract-guard audit or path-drift lane.
- dispatch one narrow cleanup task that introduces a shared test runtime lookup
  helper under `pkg/testutil` and migrates the repeated map-backed stubs that do
  not encode unique behavior.
- keep bespoke per-test doubles only where the test truly needs custom lookup
  behavior, such as a logical-move workstation-only stub or scheduler-specific
  projection behavior.

## customer asks

- `factory/logs/meta/asks.md` currently carries active asks under `release
  plans`, `system deficits`, and `quality`.
- no explicit urgency marker or top-ranked ask is recorded in the tracked meta
  ask surface.
