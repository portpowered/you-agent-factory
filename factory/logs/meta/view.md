# meta view

## world state

- as of `2026-05-03T17:03:02.5759459-07:00`, local `HEAD` on `main` points to
  `1bef2b5`
  (`align-current-selection-relationship-graph-and-dispatch-attempt-details (#77)`)
  and matches `origin/main`
- the local worktree is not clean:
  - tracked local edits exist in `factory/logs/meta/asks.md` and
    `factory/workstations/cleaner/AGENTS.md`
  - ignored local workflow residue exists under `factory/inputs/**`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`, and
  the remaining active ask lane now open on GitHub is:
  - `#69` for multi-output workstation routes

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
- the checked-in maintainer loop remains:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `consume` completes same-name `idea` + `task` pairs once both reach
  `to-complete`
- topology details that still matter:
  - `process` and `review` run in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity is `10`; each staffed workstation requests
    `1`
  - hourly `cleaner` emits `cron-triggers:complete`
  - `executor-loop-breaker` fails `task:init` after `process` visit `50`
  - `review-loop-breaker` fails `task:in-review` after `review` visit `10`

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- `.gitignore` still ignores live workflow submissions under `factory/inputs/**`
  except those sentinel paths
- the current checkout also contains ignored operating residue:
  - `factory/inputs/idea/default/workstation-non-success-route-arrays.md`
  - `factory/inputs/idea/default/remove-list-work-legacy-pagination-shim.md`
- the watcher still accepts direct `factory/inputs/<work-type>/...` paths as
  the default channel even though the public docs emphasize the
  `factory/inputs/<work_type-or-BATCH>/<channel>/<filename>` layout

## customer-ask truth

- the highest-priority live ask is the import/export P0 in
  `factory/logs/meta/asks.md`, not the older throttle cleanup lane
- the import/export P0 has materially advanced on `main`:
  - PR `#67` removed exported workstation `promptTemplate` from the public
    contract
  - PR `#68` moved worker/workstation body ownership into split body-only
    `AGENTS.md` files with a thinner authored layout
  - PR `#70` made supported bundled-file import/export round-trips disk-backed
    by default
  - PR `#71` moved the dashboard import/export dialog into direct dialog
    ownership and converged the controls onto the shared button surface
  - PR `#72` landed the checked-in import/export standards checklist and its
    highest-value gap closures
  - PR `#75` removed the extra submit-work intro copy from the dashboard card
- the local helper batch in `factory/scripts/import-export-p0-followups.json`
  is now fully stale as a dispatch surface: every work item it names has either
  already merged or been superseded by merged work
- `factory/logs/meta/asks.md` also contains a backend contract ask that is not
  represented in that helper batch:
  - replace singular workstation `onContinue`, `onRejection`, and `onFailure`
    destinations with array-based outputs
- the tracked local ask diff now makes that route-array contract ask explicit in
  the canonical backlog file, but it still remains an already-owned lane rather
  than an unowned queue gap
- the only remaining import/export P0 contract lane still open on GitHub is
  PR `#69` `workstation-non-success-route-arrays`
- the live code also shows the route-array ask is real on `main`, but it is no
  longer unowned:
  - `api/components/schemas/data-models/Workstation.yaml`,
    `api/openapi.yaml`, `pkg/api/generated/server.gen.go`,
    `pkg/generatedclient/client.gen.go`, and
    `ui/src/api/generated/openapi.ts` still expose singular
    `onContinue`/`onRejection`/`onFailure` fields beside array-valued
    `outputs`
  - `pkg/interfaces/factory_config.go`, `pkg/config/layout.go`,
    `pkg/config/factory_config_mapping.go`, and `pkg/config/config_mapper.go`
    still model non-success routes as single destinations
  - `pkg/factory/event_history.go` and
    `pkg/factory/projections/world_state.go` still collapse route arrays down
    to one public `WorkstationIO`
  - `ui/src/api/factory-definition/api.ts` still parses `onRejection` and
    `onFailure` as singular objects and currently drops `onContinue` on import
  - open PR `#69` already carries the corresponding cross-layer fix on branch
    `ralph/workstation-non-success-route-arrays`
- test coverage already exists around these seams, but it currently protects the
  old contract in several places:
  - `pkg/api/factory_config_smoke_test.go`
  - `pkg/config/factory_config_mapping_test.go`
  - `pkg/config/portable_bundled_files_test.go`
  - `tests/functional/runtime_api/api_runtime_config_alignment_smoke_test.go`
  - `tests/functional/bootstrap_portability/agent_factory_export_import_fixture_test.go`
  - `pkg/api/openapi_contract_test.go`
  - `pkg/config/config_mapper_test.go`
  - `pkg/config/config_validator_test.go`
  - `pkg/replay/event_artifact_test.go`
  - `pkg/factory/projections/world_state_test.go`
  - `pkg/cli/init/init_test.go`
  - `ui/src/api/factory-definition/api.test.ts`
  - `ui/src/features/timeline/state/factoryTimelineStore.test.ts`
- the import preview dialog ownership seam is no longer open:
  - PR `#76`
    `finish-import-preview-dialog-extraction-from-workflow-activity` merged on
    `2026-05-03`
- the selected-work current-selection ask is now materially satisfied on
  `main`:
  - PR `#74` merged the broad dispatch-centric simplification
  - PR `#77`
    `align-current-selection-relationship-graph-and-dispatch-attempt-details`
    merged on `2026-05-03`
  - `ui/src/features/current-selection/work-item-card.tsx` now renders a
    graph-shaped relationship surface instead of only a flat relationship list
  - `ui/src/features/current-selection/selected-work-dispatch-history-card.tsx`
    and
    `ui/src/features/current-selection/selected-work-dispatch-attempt-sections.tsx`
    now render nested inference and script attempt details per dispatch
  - `ui/src/features/current-selection/work-item-card.test.tsx` now locks those
    behaviors with rendered UI assertions
- with `#76` and `#77` merged, the older ignored import-preview and
  current-selection idea files were stale operating residue rather than live
  queue truth and have now been pruned locally
- the next safe non-overlapping cleanup seam is a narrow backend simplification:
  - `pkg/api/server.go` still registers a handwritten `/work` route through
    `handleListWorkWithLegacyPagination` solely to preserve tolerant legacy
    `maxResults` parsing ahead of the generated handler
  - `pkg/api/server_test.go` currently protects that tolerance with
    `TestListWork_InvalidMaxResultsDefaultsToCurrentBehavior`

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact copy
  of the current workflow contract; it predates `to-complete` states, `consume`,
  and the current `executor-slot` capacity of `10`
- replay outcome counts remain unchanged in the sample:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## recent repo movement

- recent merged PRs on `main` now include:
  - `#77`
    `align-current-selection-relationship-graph-and-dispatch-attempt-details`,
    merged on `2026-05-03`
  - `#76`
    `finish-import-preview-dialog-extraction-from-workflow-activity`, merged on
    `2026-05-03`
  - `#75` `trim-submit-work-card-intro-copy`, merged on `2026-05-03`
  - `#74` `simplify-current-selection-dispatch-detail-surface`, merged on
    `2026-05-03`
  - `#73` `retire-dashboard-button-wrapper`, merged on `2026-05-03`
  - `#72` `import-export-standards-alignment-checklist-and-gap-closure`,
    merged on `2026-05-03`
  - `#71` `dashboard-import-export-dialog-extraction-and-button-standardization`,
    merged on `2026-05-03`
  - `#70` `import-export-bundled-files-disk-backed-roundtrip`, merged on
    `2026-05-03`
  - `#68` `import-export-expanded-agents-layout-for-workers-and-workstations`
    merged on `2026-05-03`
  - `#67` `import-export-api-contract-remove-workstation-prompt-template`
    merged on `2026-05-03`
  - `#66` `add fixes for edges missing` merged on `2026-05-03`
  - `#65` `retire-dashboard-format-helper-ownership`
  - `#64` `retire-dashboard-bento-layout-ownership`
  - `#63` `retire-current-selection-inference-duplication`
  - `#62` `align-dashboard-work-summary-count-semantics`
- the only open PR directly tied to an active ask lane is:
  - `#69` `workstation-non-success-route-arrays`, opened on `2026-05-03`

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- helper planning residue can go stale quickly once related PRs merge, so the
  meta loop has to reconcile ignored backlog files against `main` and open PR
  state before dispatching anything new
- import/export prompt-template, split-layout, bundled-file, dialog,
  standards-alignment, and the remaining import-preview ownership move are now
  landed on `main`
- the route-array contract cleanup is already actively owned by ignored local
  residue plus open PR `#69`, so queuing another idea for it would be
  duplicative
- the broad current-selection simplification ask is now merged on `main`, so
  the ignored local idea file for that lane was stale operating residue rather
  than live queue truth and has now been pruned locally
- the earlier local button-wrapper idea has already been consumed by merged
  PR `#73`, so keeping that ignored idea file around would only create stale
  local queue residue
- broad ask ownership and narrow ask completeness are different checks, but
  merged PR `#77` closes the literal selected-work interpretation gap that
  earlier notes still treated as open
- once the remaining P0 route-array lane is already owned, the best safe
  dispatch is a narrow simplification outside `#69` rather than a second
  overlapping import/export contract request
- the right meta action in this iteration is to refresh the checked-in world
  view, record that PR `#76` and PR `#77` have merged, prune the now-stale
  local current-selection/import-export helper residue, and queue one narrow
  backend cleanup idea that retires the handwritten list-work legacy pagination
  shim
