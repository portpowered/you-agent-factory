# meta view

## world state

- as of `2026-05-03T09:02:23.4813925-07:00`, local `HEAD` on `main` points to
  `c9bcbcf` (`Merge pull request #68 from
  portpowered/ralph/import-export-expanded-agents-layout-for-workers-and-workstations`)
  and matches `origin/main`; there is one open PR relevant to the active ask:
  `#69` `workstation-non-success-route-arrays`
- the local worktree is not clean:
  - tracked local edits exist in `factory/logs/meta/asks.md` and
    `factory/workstations/cleaner/AGENTS.md`
  - untracked local planning residue exists in
    `factory/scripts/import-export-p0-followups.json`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`, and
  the active P0 remains import/export and contract cleanup, now expanded with a
  separate multi-output workstation-route ask

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
  - `factory/inputs/idea/default/workstation-non-success-route-arrays.md`
- the local repository root also contains untracked planning residue outside the
  canonical inboxes:
  - `factory/scripts/import-export-p0-followups.json`
- the watcher still accepts direct `factory/inputs/<work-type>/...` paths as
  the default channel even though the public docs emphasize the
  `factory/inputs/<work_type-or-BATCH>/<channel>/<filename>` layout

## customer-ask truth

- the highest-priority live ask is the import/export P0 in
  `factory/logs/meta/asks.md`, not the older throttle cleanup lane
- two earlier import/export cleanup requests have now landed on `main`:
  - PR `#67` removed exported workstation `promptTemplate` from the public
    contract
  - PR `#68` moved worker/workstation body ownership into split body-only
    `AGENTS.md` files with a thinner authored layout
- the local helper batch in `factory/scripts/import-export-p0-followups.json`
  is now partially stale: it still lists the prompt-template and split-layout
  lanes even though those changes are merged, while its bundled-file, dialog,
  and standards items remain relevant
- `factory/logs/meta/asks.md` also contains a backend contract ask that is not
  represented in that helper batch:
  - replace singular workstation `onContinue`, `onRejection`, and `onFailure`
    destinations with array-based outputs
- the remaining import/export seams on `main` are now narrower:
  - `pkg/config/portable_bundled_files.go` auto-collects supported bundled
    files during flatten, but `pkg/config/factory_config_mapping.go` still
    serializes bundled-file inline content into the exported API shape
  - the dashboard import/export dialog and button-standardization lane remains
    open in the ask surface and is not yet represented by a merged PR
  - the standards-alignment checklist lane remains open and is still tracked in
    `factory/logs/meta/progress.txt`
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
  - `#68` `import-export-expanded-agents-layout-for-workers-and-workstations`
    merged on `2026-05-03`
  - `#67` `import-export-api-contract-remove-workstation-prompt-template`
    merged on `2026-05-03`
  - `#66` `add fixes for edges missing` merged on `2026-05-03`
  - `#65` `retire-dashboard-format-helper-ownership`
  - `#64` `retire-dashboard-bento-layout-ownership`
  - `#63` `retire-current-selection-inference-duplication`
  - `#62` `align-dashboard-work-summary-count-semantics`
- there is one open PR directly tied to the remaining P0 contract cleanup:
  - `#69` `workstation-non-success-route-arrays`, opened on `2026-05-03`

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- helper planning residue can go stale quickly once related PRs merge, so the
  meta loop has to reconcile ignored backlog files against `main` and open PR
  state before dispatching anything new
- the prompt-template and split-layout asks are now landed on `main`; the
  remaining import/export backlog is bundled files, dialog/button cleanup,
  standards alignment, and the array-based non-success route contract
- the route-array contract cleanup is already actively owned by ignored local
  residue plus open PR `#69`, so queuing another idea for it would be
  duplicative
- the right durable meta action in this iteration is to refresh the checked-in
  world view and progress surfaces only, leaving execution with the existing
  import/export branches and PRs
