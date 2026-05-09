# meta view

## world state

- as of `2026-05-07T05:03:44.1967849-07:00`, local `HEAD` on `main` points to
  `3d0c461` (`docs: refresh meta world state`), is ahead of `origin/main` by
  three local meta commits, and already contains upstream `origin/main` through
  merged PR `#147` (`localize-export-dialog-and-trigger`) at `6451e25`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the only tracked local dirtiness outside this meta refresh remains the
  user-maintained canonical ask file `factory/logs/meta/asks.md`
- canonical `factory/inputs/**` remains tracked-sentinel-only in git, while
  live maintainer submissions remain ignored operating state

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
- the checked-in maintainer loop remains:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:complete`
- topology details that still matter:
  - `process` and `review` run in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity is `10`
  - loop breakers still guard repeated `process` and `review` retries

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- `.gitignore` still keeps live workflow submissions under `factory/inputs/**`
  out of normal commits except for those sentinel paths
- the ignored idea files
  `factory/inputs/idea/default/localize-export-dialog-and-trigger.md` and
  `factory/inputs/idea/default/narrow-cron-watcher-runtime-lookup-contract.md`
  were stale on live `main` because merged PR `#147` landed on
  `2026-05-07T11:46:43Z` and merged PR `#146` landed on
  `2026-05-07T11:39:54Z`
- after pruning those stale residues, the maintainer-owned ignored queue now
  carries three fresh non-overlapping idea files so the autonomous lane still
  matches the standing ask to keep at least three tasks running:
  - `audit-repository-against-2026-website-and-backend-checklists.md`
  - `retire-template-fields-variadic-worktree-shim.md`
  - `localize-dashboard-header-timeline-and-stream-status.md`

## customer-ask truth

- the canonical ask file still carries one broad active quality lane plus the
  autonomy notice through `2026-05-25`
- the active quality asks remain:
  - follow the external website and backend checklists and create alignment
    tasks
  - keep backend and website testing moving toward declared high-coverage goals
  - keep simplifying backend and website ownership where duplicate or stale
    logic remains
  - keep at least three non-overlapping tasks running at a time
- the external checklist links in `factory/logs/meta/asks.md` still point at
  the live `portpowered/checklists` repository, and the explicit linked docs
  remain the current `2026` checklist revisions:
  - `website-development-checklist.md`
  - `backend-development-checklist.md`
- the linked external `asks.md` source still has an evidence gap on
  `2026-05-07`: `https://raw.githubusercontent.com/portpowered/checklists/main/asks.md`
  does not resolve to a readable checklist document, so that ask reference
  still needs to be treated as unavailable evidence rather than assumed input
- there is still no merged checked-in repo-wide review record mapping this
  repository against those external checklist documents on live `main`; open
  PR `#141` currently owns that audit lane
- the UI localization foothold now reaches both import and export surfaces on
  live `main`:
  - `ui/src/i18n/index.ts`, `ui/src/i18n/locales.ts`, and
    `ui/src/i18n/messages.ts` exist on `main`
  - merged PR `#142` localized and accessibility-hardened the import preview
    dialog
  - merged PR `#147` localized the export dialog and export trigger
  - `ui/src/features/header/tick-slider-control.tsx` and
    `ui/src/features/header/dashboard-header.tsx` still keep concentrated
    hardcoded English timeline and stream-status accessibility text, which is
    now the next narrow header-local follow-up

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## recent repo movement

- recent merged PRs on `main` now include:
  - `#147` `localize-export-dialog-and-trigger`, merged on
    `2026-05-07T11:46:43Z`
  - `#146` `narrow-cron-watcher-runtime-lookup-contract`, merged on
    `2026-05-07T11:39:54Z`
  - `#144` `cover-releasesmoke-command-owner-parse-and-json-error-branches`,
    merged on `2026-05-07T10:13:02Z`
  - `#142` `localize-and-accessibility-harden-import-preview-dialog`, merged
    on `2026-05-07T09:34:50Z`
  - `#140` `retire-deadcodecheck-gotypesalias-compat-shim`, merged on
    `2026-05-07T09:25:03Z`
- `gh pr list --state open` now reports:
  - `#145` `docs: refresh meta world state`
  - `#143` `docs: refresh meta world state`
  - `#141` `audit-repository-against-2026-website-and-backend-checklists`
  - `#139` `docs: refresh meta world state`
  - `#123` `docs: refresh meta world state`
  - `#120` `docs: refresh meta world state`
- open PR `#141` already owns the current checklist-audit lane, so new backlog
  replacements must avoid its doc surface while still acting on live code gaps

## next cleanup candidates

- the repo-wide standards audit remains the highest-priority checklist ask on
  live `main`, but PR `#141` already owns that documentation lane
- the next narrow backend simplification seam is the workers template-field
  helper:
  - `pkg/workers/template_fields.go` still exposes `ResolveTemplateFields`
    with an explicitly backwards-compat variadic `worktreeTemplate ...string`
    parameter
  - the production caller in `pkg/workers/workstation_executor.go` already
    passes one concrete `workstationDef.Worktree` value, so the legacy
    compatibility shape is no longer needed on the live path
  - the direct fallout is localized to `pkg/workers/template_fields_test.go`
    plus the one production caller, making this a small implementation-ready
    shim retirement
- the next narrow UI checklist-alignment seam is the dashboard header control
  surface:
  - `ui/src/features/header/tick-slider-control.tsx` still hardcodes the slider
    label, slider `aria-label`, waiting-state text, tick status text, and
    "Return to current tick" button name
  - `ui/src/features/header/dashboard-header.tsx` still hardcodes the region
    label and stream-status accessible names for `live`, `offline`, and
    `connecting`
  - the import/export dialog localization pattern already exists on live
    `main`, so this is now a small feature-local follow-up rather than a new
    i18n foundation project
- the next backup backend seam after that is `cmd/releaseprep`:
  - `go test -cover ./cmd/...` now reports `94.1%` for `cmd/releaseprep`
  - `cmd/releaseprep/main.go` still leaves the explicit `flag.ErrHelp` success
    branch and direct parse-failure routing as a small command-owner coverage
    closeout
- the next backup backend testing seam after that is still `cmd/gocoveragecheck`:
  - `go test -cover ./cmd/...` now reports `75.0%` for
    `cmd/gocoveragecheck`
  - remaining coverage is concentrated in command-owner execution, package-list,
    and coverage-evaluation error branches rather than in backend application
    packages

## theory of mind

- the authoritative world model comes from live `main`, the checked-in workflow
  contract, the canonical ask file, current PR state, and current external
  checklist docs together
- `factory/inputs/**` must still be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- when a previously queued idea lands on `main`, prune the ignored residue and
  refresh the next seam from live code and PR state before queuing anything
  else; merged PRs `#146` and `#147` invalidated two of the three active
  backlog slots within the same cycle
- when the customer ask requires at least three tasks in flight, satisfy that
  ask with three narrow non-overlapping idea files rather than one broad batch
  unless dependency ordering is actually required
- when checklist conformance is still undocumented, queue one audit lane plus
  smaller implementation-ready follow-ups instead of claiming alignment from
  standards intent alone
- when a localization follow-up merges on one dashboard dialog, re-read the
  adjacent header controls and accessibility labels next; the concentrated
  English-only residue often lives there rather than in a second dialog
- when a helper still advertises an explicitly backwards-compatible variadic or
  optional parameter but the live production caller already passes the
  canonical explicit shape, retire the shim before widening into larger runtime
  refactors
- when a repo-owned command package is already above 90% coverage, prefer the
  remaining help, parse, or exit-routing branches in that thin command owner
  before widening into the underlying internal package
- when the linked external checklist repo omits one requested source such as
  `asks.md`, record that as an evidence gap and continue from the sources that
  are actually retrievable instead of inventing missing checklist content
