# canonicalize-workspace-setup-on-python3

## Why

The checked-in repository workflow currently has a live portability seam at the
`setup-workspace` boundary.

Current live evidence on `main`:

- `factory/workers/workspace-setup/AGENTS.md` still hard-codes
  `command: python` for the repository-owned workspace setup worker
- the current machine has `python3` available but no `python`
- the active queued idea
  `split-functionallong-provider-template-helpers-from-default-support`
  already hit that exact failure path in
  `ui/integration/fixtures/terminal-summary-regression-replay.jsonl`:
  the accepted idea dispatches `setup-workspace`, then fails with
  `exec: "python3": executable file not found in $PATH`
- the same `python` contract is still mirrored in the repository-owned workflow
  test fixtures under:
  - `tests/adhoc/factory/workers/workspace-setup/AGENTS.md`
  - `tests/functional_test/testdata/idea_plan_execute_review_with_limits/workers/workspace-setup/AGENTS.md`

This is a narrow cleanup and unblocker seam: the repository already ships the
workspace setup script as a Python 3 script, but the checked-in worker contract
still depends on the legacy `python` alias.

## Do

- inspect the checked-in `setup-workspace` worker contract and switch the
  repository-owned workflow to one canonical Python 3 interpreter contract
  instead of the current `python` alias assumption
- update only the repository-owned mirrors and verification surfaces that prove
  that contract, such as the checked-in worker fixture/testdata copies and any
  config or event-history assertions that still require the old command string
- preserve the intended public workflow behavior:
  idea `plan:init` should still advance through `setup-workspace` into
  `task:init`; only the interpreter contract should change
- keep test verification behavioral where possible:
  prefer workflow, config, replay, or emitted-event assertions that prove the
  checked-in `setup-workspace` path still runs correctly, rather than adding
  new source-layout checks

## Constraints

- do not broaden this lane into general multi-interpreter fallback logic or a
  wider script-runner redesign
- do not change unrelated worker contracts, factory runtime semantics, or the
  provider-helper cleanup lane
- prefer removing the repository-owned dependency on the legacy `python` alias
  over adding another compatibility layer
- keep changes focused on the checked-in repository workflow, its owned
  fixtures, and the minimum docs/config surfaces that must stay aligned

## Verification

- run the narrow tests that cover the checked-in `setup-workspace` worker
  contract, config mapping, and script-copy behavior
- run the relevant replay or workflow coverage that proves a queued idea can
  advance past `setup-workspace` without the missing-`python` failure
