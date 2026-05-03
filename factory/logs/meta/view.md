# meta view

## world state

- as of `2026-05-03T08:05:06.9683608-07:00`, local `HEAD` on `main` points to
  `0393c72` (`update the ideafy instructions`) while `origin/main` still points
  to `1852bc1` (`docs: refresh meta world state`); there are no open PR
  branches visible through `gh pr list --state open`
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
- the local repository root also contains untracked planning residue outside the
  canonical inboxes:
  - `factory/scripts/import-export-p0-followups.json`
- the watcher still accepts direct `factory/inputs/<work-type>/...` paths as
  the default channel even though the public docs emphasize the
  `factory/inputs/<work_type-or-BATCH>/<channel>/<filename>` layout

## customer-ask truth

- the highest-priority live ask is the import/export P0 in
  `factory/logs/meta/asks.md`, not the older throttle cleanup lane
- the local helper batch in `factory/scripts/import-export-p0-followups.json`
  already decomposes most of that P0 into five ordered cleanup ideas:
  - remove exported workstation `promptTemplate`
  - push worker/workstation body ownership fully into split `AGENTS.md` files
  - make bundled files disk-backed by default
  - extract import/export dialogs and standardize button styling
  - track and close import/export standards gaps
- `factory/logs/meta/asks.md` now also contains a new backend contract ask that
  is not represented in that helper batch:
  - replace singular workstation `onContinue`, `onRejection`, and `onFailure`
    destinations with array-based outputs
- the live code still shows the main backend contract seams behind that batch:
  - `api/openapi.yaml`, `api/components/schemas/data-models/Workstation.yaml`,
    `pkg/api/generated/server.gen.go`, and
    `pkg/config/factory_config_mapping.go` still expose workstation
    `promptTemplate`
  - `pkg/config/layout.go` still writes expanded worker/workstation `AGENTS.md`
    through `renderAgentsMarkdown(...)`, preserving frontmatter-driven files
    instead of body-only prompt ownership
  - `pkg/config/portable_bundled_files.go` auto-collects supported bundled
    files during flatten, but `pkg/config/factory_config_mapping.go` still
    serializes bundled-file inline content into the exported API shape
- the live code also shows the new route-array ask is real and still open:
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
  - `#66` `add fixes for edges missing` merged on `2026-05-03`
  - `#65` `retire-dashboard-format-helper-ownership`
  - `#64` `retire-dashboard-bento-layout-ownership`
  - `#63` `retire-current-selection-inference-duplication`
  - `#62` `align-dashboard-work-summary-count-semantics`
  - `#61` `browser-shared-action-primitives`
  - `#60` `browser-integration-png-export-import-roundtrip`
- `main` also moved through two direct post-merge commits relevant to the meta
  loop:
  - `ce8ca55` `fix the factory definition to be able to import the config`
  - `c82b118` `update the requirements for the ideafy`
- local `main` has advanced one unpublished commit past `origin/main`:
  - `0393c72` `update the ideafy instructions`

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- the import/export P0 is only partially decomposed locally:
  the helper batch covers the prompt-template, split-layout, bundled-file,
  dialog, and standards lanes, but not the newly added array-based
  continue/rejection/failure contract cleanup
- the correct next move in this iteration is to refresh the checked-in world
  view and queue one narrow standalone cleanup idea for the unqueued
  multi-output route contract instead of duplicating the existing import/export
  helper batch
- the right durable meta action in this iteration is to refresh the checked-in
  world view and standards checklist while the local import/export queue owns
  execution
