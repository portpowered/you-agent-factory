# meta view

## world state

- as of `2026-05-07T04:04:43.0744927-07:00`, local `HEAD` on `main` points to
  `b145e56` (`Merge branch 'main' of https://github.com/portpowered/infinite-you`)
  and already contains upstream `origin/main` through merged PR `#144`
  (`cover-releasesmoke-command-owner-parse-and-json-error-branches`) at
  `8a7f0f9`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- before this refresh, the only visible tracked local edit was the
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
  `factory/inputs/idea/default/localize-and-accessibility-harden-import-preview-dialog.md`
  and
  `factory/inputs/idea/default/cover-releasesmoke-command-owner-parse-and-json-error-branches.md`
  are now stale on live `main` because merged PR `#142` landed on
  `2026-05-07T09:34:50Z` and merged PR `#144` landed on
  `2026-05-07T10:13:02Z`
- after pruning those stale residues, the maintainer-owned ignored queue should
  carry three fresh non-overlapping idea files so the autonomous lane matches
  the standing ask to keep at least three tasks running:
  - `audit-repository-against-2026-website-and-backend-checklists.md`
  - `localize-export-dialog-and-trigger.md`
  - `narrow-cron-watcher-runtime-lookup-contract.md`

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
- the UI now has a shared localization foothold on live `main`:
  - `ui/src/i18n/index.ts`, `ui/src/i18n/locales.ts`, and
    `ui/src/i18n/messages.ts` now exist on `main`
  - `ui/package.json` now carries `jest-axe`, but broad feature-local
    accessibility automation is still not wired through the dashboard surface
  - merged PR `#142` only localized and accessibility-hardened the import
    preview dialog, so adjacent export-trigger and export-dialog copy remains
    the next narrow i18n follow-up

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
  - `#144` `cover-releasesmoke-command-owner-parse-and-json-error-branches`,
    merged on `2026-05-07T10:13:02Z`
  - `#142` `localize-and-accessibility-harden-import-preview-dialog`, merged
    on `2026-05-07T09:34:50Z`
  - `#140` `retire-deadcodecheck-gotypesalias-compat-shim`, merged on
    `2026-05-07T09:25:03Z`
  - `#138` `Branchling`, merged on `2026-05-07T08:23:18Z`
  - `#137` `cover-gocoveragecheck-helper-runtime-and-parser-branches`, merged
    on `2026-05-07T03:23:26Z`
- `gh pr list --state open` now reports:
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
- the next narrow UI checklist-alignment seam is the export path:
  - `ui/src/features/export/export-factory-dialog.tsx` still hardcodes the
    export dialog title, body copy, labels, validation text, success text, and
    action labels
  - `ui/src/features/header/dashboard-header.tsx` still hardcodes the export
    trigger `aria-label`
  - the shared dialog already supports `closeLabel`, but the export dialog does
    not yet pass localized close-control copy
- the next narrow backend simplification seam is the cron watcher runtime
  contract:
  - `pkg/interfaces/runtime_lookup.go` still exposes
    `RuntimeConfigLookup` as a composite of definition lookups plus
    path-aware methods
  - `pkg/service/cron_watcher.go` currently accepts
    `interfaces.RuntimeConfigLookup` across its top-level cron helpers even
    though most of that file only needs workstation lookup plus a separately
    provided workflow identity
  - narrowing cron-side signatures to `RuntimeWorkstationLookup` is the
    smallest non-overlapping interface cleanup now that the canonical lookup
    owner is already centralized in `pkg/interfaces`
- the next backup backend testing seam after that is `cmd/releaseprep`:
  - `go test -cover ./cmd/...` now reports `94.1%` for `cmd/releaseprep`
  - `cmd/releaseprep/main.go` still leaves the explicit `flag.ErrHelp` branch
    in `run()` without direct package-local coverage

## theory of mind

- the authoritative world model comes from live `main`, the checked-in workflow
  contract, the canonical ask file, current PR state, and current external
  checklist docs together
- `factory/inputs/**` must still be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- when a previously queued idea lands on `main`, prune the ignored residue and
  refresh the next seam from live code and PR state before queuing anything
  else; merged PR `#140` invalidated one of the three active backlog slots
  within the same cycle
- when the customer ask requires at least three tasks in flight, satisfy that
  ask with three narrow non-overlapping idea files rather than one broad batch
  unless dependency ordering is actually required
- when checklist conformance is still undocumented, queue one audit lane plus
  smaller implementation-ready follow-ups instead of claiming alignment from
  standards intent alone
- when one queued lane merges while sibling PRs are still open, replace only
  the merged slot with a new non-overlapping idea instead of re-queuing work
  already owned by the open PRs
- when the linked external checklist repo omits one requested source such as
  `asks.md`, record that as an evidence gap and continue from the sources that
  are actually retrievable instead of inventing missing checklist content
