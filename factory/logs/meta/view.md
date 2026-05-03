# meta view

## world state

- as of `2026-05-03T07:03:16.3903787-07:00`, `HEAD` on `main` points to
  `c82b118` (`update the requirements for the ideafy`), matching
  `origin/main`; the worktree is clean and there are no open PR branches
  visible through `gh pr list --state open`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`, and
  the active P0 has shifted from throttle cleanup to import/export contract and
  UX cleanup

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
- the current checkout also contains ignored operating residue under the
  canonical inboxes:
  - `factory/inputs/BATCH/default/import-export-p0-followups.json`
  - `factory/inputs/thoughts/default/import-export-issues.md`
- the watcher still accepts direct `factory/inputs/<work-type>/...` paths as
  the default channel even though the public docs emphasize the
  `factory/inputs/<work_type-or-BATCH>/<channel>/<filename>` layout

## customer-ask truth

- the highest-priority live ask is the import/export P0 in
  `factory/logs/meta/asks.md`, not the older throttle cleanup lane
- the ignored local batch already decomposes that P0 into five ordered cleanup
  ideas:
  - remove exported workstation `promptTemplate`
  - push worker/workstation body ownership fully into split `AGENTS.md` files
  - make bundled files disk-backed by default
  - extract import/export dialogs and standardize button styling
  - track and close import/export standards gaps
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
- test coverage already exists around these seams, but it currently protects the
  old contract in several places:
  - `pkg/api/factory_config_smoke_test.go`
  - `pkg/config/factory_config_mapping_test.go`
  - `pkg/config/portable_bundled_files_test.go`
  - `tests/functional/runtime_api/api_runtime_config_alignment_smoke_test.go`
  - `tests/functional/bootstrap_portability/agent_factory_export_import_fixture_test.go`

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

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- the import/export P0 is already decomposed locally into a batch that matches
  the active customer ask closely enough that the correct next move is to avoid
  queuing a duplicate backlog item this pass
- the right durable meta action in this iteration is to refresh the checked-in
  world view and standards checklist while the local import/export queue owns
  execution
