# meta view

## world state

- as of `2026-05-03T16:02:24.9115382-07:00`, local `HEAD` on `main` points to
  `f803e8d` (`trim-submit-work-card-intro-copy (#75)`)
  and matches `origin/main`
- the local worktree is not clean:
  - tracked local edits exist in `factory/logs/meta/asks.md` and
    `factory/workstations/cleaner/AGENTS.md`
  - untracked local planning residue exists in
    `factory/scripts/import-export-p0-followups.json`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`, and
  the remaining active ask lanes now open on GitHub are:
  - `#69` for multi-output workstation routes
  - `#76` for the remaining import preview dialog ownership extraction

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
- the current checkout also contains ignored operating residue related to the
  active ask:
  - `factory/inputs/thoughts/default/import-export-issues.md`
  - `factory/inputs/idea/default/align-current-selection-relationship-graph-and-dispatch-attempt-details.md`
  - `factory/inputs/idea/default/workstation-non-success-route-arrays.md`
  - `factory/inputs/idea/default/finish-import-preview-dialog-extraction-from-workflow-activity.md`
- the local repository root also contains untracked planning residue outside the
  canonical inboxes:
  - `factory/scripts/import-export-p0-followups.json`
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
- the broad current-selection simplification ask has now merged on `main` as
  PR `#74` `simplify-current-selection-dispatch-detail-surface`
- the selected-work view on `main` is now materially dispatch-centric:
  - `ui/src/features/current-selection/work-item-card.tsx` limits the primary
    work-item surface to summary fields, relationship listings, and dispatch
    history
  - `ui/src/features/current-selection/selected-work-dispatch-history.tsx` and
    `ui/src/features/current-selection/selected-work-dispatch-history-card.tsx`
    now carry the surviving request/response inspection surface
- one narrower interpretation gap remains after the broad simplification merge:
  - the ask literally describes a relationship graph and a nested per-dispatch
    inference-attempt list, while `main` currently renders a textual
    relationship list and a consolidated per-dispatch request/response view
- the import dialog ownership seam is no longer unowned:
  - open PR `#76`
    `finish-import-preview-dialog-extraction-from-workflow-activity` now owns
    the remaining wrapper-removal and dialog-home cleanup
- the next narrow unowned UI cleanup is the literal current-selection
  interpretation gap:
  - `ui/src/features/current-selection/work-item-card.tsx` still shows
    relationships as a textual list instead of a graph-shaped surface
  - `ui/src/features/current-selection/selected-work-dispatch-history.tsx` and
    `ui/src/features/current-selection/selected-work-dispatch-history-card.tsx`
    still show consolidated dispatch request/response details instead of a
    nested per-dispatch attempt list

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
- the open PRs directly tied to active ask lanes are:
  - `#69` `workstation-non-success-route-arrays`, opened on `2026-05-03`
  - `#76` `finish-import-preview-dialog-extraction-from-workflow-activity`,
    opened on `2026-05-03`

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- helper planning residue can go stale quickly once related PRs merge, so the
  meta loop has to reconcile ignored backlog files against `main` and open PR
  state before dispatching anything new
- import/export prompt-template, split-layout, bundled-file, dialog, and
  standards-alignment asks are now landed on `main`, while the final import
  preview ownership move is already owned by open PR `#76`
- the route-array contract cleanup is already actively owned by ignored local
  residue plus open PR `#69`, so queuing another idea for it would be
  duplicative
- the broad current-selection simplification ask is now merged on `main`, so
  the ignored local idea file for that lane was stale operating residue rather
  than live queue truth and has now been pruned locally
- the earlier local button-wrapper idea has already been consumed by merged
  PR `#73`, so keeping that ignored idea file around would only create stale
  local queue residue
- broad ask ownership and narrow ask completeness are different checks:
  a merged change can satisfy the operational problem even if one literal ask
  detail still looks under-served
- the narrow remaining seam visible after reconciling merged PR `#74` is no
  longer duplicate top-level panels; it is the gap between the ask's literal
  request for a relationship graph plus per-attempt inference lists and the
  simpler merged textual relationship list plus consolidated dispatch card
- once the remaining P0 route-array lane is already owned, the best safe
  dispatch is a narrow, isolated follow-up from the canonical ask surface rather
  than a second overlapping import/export contract request
- the right meta action in this iteration is to refresh the checked-in world
  view, record that PR `#75` has merged, record that PR `#76` now owns the
  remaining import preview extraction seam, and queue the literal
  current-selection graph-plus-attempt-detail follow-up as a standalone ignored
  idea
