# meta view

## world state

- as of `2026-05-07T03:02:15.3916934-07:00`, local `HEAD` on `main` points to
  `41ae281` (`Merge branch 'main' of https://github.com/portpowered/infinite-you`)
  and already contains upstream `origin/main` through merged PR `#140`
  (`retire-deadcodecheck-gotypesalias-compat-shim`) at `08acc2f`
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
- the previously queued ignored idea
  `factory/inputs/idea/default/retire-deadcodecheck-gotypesalias-compat-shim.md`
  is now stale on live `main` because merged PR `#140` landed that exact lane
  on `2026-05-07T09:34:50Z`
- after pruning that stale residue, the maintainer-owned ignored queue should
  carry three fresh non-overlapping idea files so the autonomous lane matches
  the standing ask to keep at least three tasks running:
  - `audit-repository-against-2026-website-and-backend-checklists.md`
  - `localize-and-accessibility-harden-import-preview-dialog.md`
  - `cover-releasesmoke-command-owner-parse-and-json-error-branches.md`

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
  - `asks.md`
- there is still no checked-in repo-wide review record mapping this repository
  against those external checklist documents on live `main`; PR `#141`
  currently owns that audit lane, but it is not merged yet
- the UI still lacks shared localization infrastructure on live `main`:
  - `ui/src/i18n` does not exist
  - `ui/package.json` does not currently carry an automated accessibility test
    dependency such as `axe-core` or a wrapper matcher
  - PR `#142` owns the first narrow import-preview lane for those gaps, but it
    is not merged yet

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
  - `#140` `retire-deadcodecheck-gotypesalias-compat-shim`, merged on
    `2026-05-07T09:34:50Z`
  - `#138` `Branchling`, merged on `2026-05-07T08:30:28Z`
  - `#137` `cover-gocoveragecheck-helper-runtime-and-parser-branches`, merged
    on `2026-05-07T03:38:27Z`
  - `#136` `Windows release`, merged on `2026-05-07T01:38:06Z`
  - `#135` `cover-functionallane-command-owner-error-and-entrypoint-branches`,
    merged on `2026-05-06T23:28:08Z`
- `gh pr list --state open` now reports:
  - `#142` `localize-and-accessibility-harden-import-preview-dialog`
  - `#141` `audit-repository-against-2026-website-and-backend-checklists`
  - `#139` `docs: refresh meta world state`
  - `#123` `docs: refresh meta world state`
  - `#120` `docs: refresh meta world state`
- PRs `#141` and `#142` already own the current checklist-audit and
  import-preview standards lanes, so the next replacement task must avoid
  those file and behavior surfaces

## next cleanup candidates

- the repo-wide standards audit remains the highest-priority checklist ask on
  live `main`, but PR `#141` already owns that documentation lane
- the narrowest current UI standards lane remains the import preview dialog,
  but PR `#142` already owns that feature-local localization and accessibility
  work
- the next non-overlapping repo-owned backend testing seam is now in
  `cmd/releasesmoke`:
  - merged PR `#128` already covered the thin `main()` entrypoint routing
  - `go test -cover ./cmd/...` still reports `83.3%` statement coverage for
    `cmd/releasesmoke`
  - `cmd/releasesmoke/main.go` still carries uncovered parse-error and
    `writeJSON` encode-failure branches in the command-owner boundary
  - `cmd/releasesmoke/main_test.go` already provides a focused package-local
    seam to add those assertions without widening into release harness changes

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
