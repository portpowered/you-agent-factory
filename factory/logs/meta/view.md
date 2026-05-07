# meta view

## world state

- as of `2026-05-06T20:04:20.2572074-07:00`, local `HEAD` on `main` points to
  `22e20d8` (`Merge pull request #136 from portpowered/windows-release`) and is
  current with `origin/main`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the tracked worktree is not clean, but the visible tracked edits are outside
  the canonical meta surfaces and should be treated as user-owned local state:
  - `examples/idea-plan-code-review/scripts/setup-workspace.py`
  - `examples/thought-idea--plan-work-review/scripts/setup-workspace.py`
  - `factory/scripts/setup-workspace.py`
  - `tests/adhoc/factory/scripts/setup-workspace.py`
  - `tests/functional_test/testdata/idea_plan_execute_review_with_limits/scripts/setup-workspace.py`
- canonical `factory/inputs/**` remains tracked-sentinel-only and there is no
  checked-in cleanup request currently queued under `factory/inputs/**`

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
- the visible canonical idea inbox is empty on this checkout aside from
  `.gitkeep`, so the next dispatch must create fresh ignored operating state
  rather than reconcile stale local residue

## customer-ask truth

- the canonical ask file still carries one broad active quality lane plus the
  autonomy notice through `2026-05-25`
- the active quality asks remain:
  - follow the external website and backend checklists and create alignment
    tasks
  - keep backend and website testing moving toward declared high-coverage goals
  - keep simplifying backend and website ownership where duplicate or stale
    logic remains
- the external checklist links in `factory/logs/meta/asks.md` still point at
  the live `portpowered/checklists` repository, and both documents are current
  `2026` checklist revisions:
  - `website-development-checklist.md`
  - `backend-development-checklist.md`
- there is still no checked-in repo-wide review record mapping this repository
  against those external checklist documents; the only checked-in alignment
  checklist found this turn is the narrower import/export lane record at
  `docs/internal/development/import-export-standards-alignment-checklist.md`

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
  - `#136` `Windows release`, merged on `2026-05-07T01:38:06Z`
  - `#135` `cover-functionallane-command-owner-error-and-entrypoint-branches`,
    merged on `2026-05-06T23:28:08Z`
  - `#134` `cover-gocoveragecheck-command-owner-threshold-and-entrypoint-branches`,
    merged on `2026-05-06T22:18:52Z`
  - `#133` `cover-releasetagcheck-git-tag-wrapper-branches`, merged on
    `2026-05-06T21:11:35Z`
  - `#132` `cover-deadcodecheck-command-owner-branches`, merged on
    `2026-05-06T20:17:42Z`
- `gh pr list --state open` still reports only the two older meta-refresh PRs:
  - `#123` `docs: refresh meta world state`
  - `#120` `docs: refresh meta world state`
- those open PRs do not own the next code cleanup lane

## next cleanup candidate

- merged PR `#135` closes the previously recorded `cmd/functionallane`
  command-owner seam on live `main`; `go test -cover ./cmd/functionallane` now
  reports `100.0%` statement coverage
- the next non-overlapping maintainer-owned quality seam is back in
  `cmd/gocoveragecheck`:
  - `make test-coverage-go` still routes through the repo-owned
    `cmd/gocoveragecheck` entrypoint
  - package coverage is still only `78.5%` on live `main`, materially below
    adjacent maintainer-gate commands
  - merged PR `#134` already covered the threshold and direct entrypoint
    branches, but `run()`, `resolveCoverageLane()`, `listGoPackages()`,
    `parseTotalCoverage()`, `evaluateCoverage()`, `parseCoverageProfile()`, and
    `coverageImportPath()` still have uncovered helper and failure branches
  - the remaining useful work is still narrow and package-local: add direct
    tests for temp-profile lifecycle behavior, coverage-tool failure detail
    selection, package-discovery and repo-root failure wrappers, and malformed
    profile or path parsing cases without changing coverage policy, thresholds,
    package selection, or CI wiring

## theory of mind

- the authoritative world model comes from live `main`, the checked-in workflow
  contract, the canonical ask file, and current PR state together
- `factory/inputs/**` must still be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- when a previously queued narrow coverage seam lands on `main`, refresh the
  next seam from live coverage output before reusing the old theory; the
  `functionallane` lane went from `82.0%` to `100.0%` within one refresh cycle
- when the customer ask references external checklist repositories, re-check
  the live linked documents before claiming conformance status; broad checklist
  intent is not evidence
- when no checked-in repo-wide checklist audit exists yet, treat checklist
  conformance as still open and queue either a narrow audit task or a smaller
  maintainer-owned enforcement seam instead of declaring the ask satisfied
- when broad quality asks remain open, prefer the next narrow repo-owned gate
  that improves reviewable evidence without overlapping unrelated user-owned
  tracked edits
