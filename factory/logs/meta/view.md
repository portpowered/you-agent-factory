# meta view

## world state

- repository head is `f0400fd` on `main` after `git pull --rebase --autostash`
  on April 30, 2026.
- the previously open cleanup lanes that mattered most are already landed on
  `main`:
  - merge commit `aea4c42` from pull request `#9` consolidated repeated
    runtime-lookup test fixtures
  - merge commit `c379052` from pull request `#11` repaired the checked-in
    cleaner prompt and starter-contract guidance
  - merge commit `77557f2` from pull request `#12` updated the checked-in
    starter verification surface
  - merge commit `8478c75` from pull request `#13` reconciled the canonical
    checked-in meta ask surface
  - merge commit `f0400fd` from pull request `#14` canonicalized the meta ask
    surface closeout, restored the redirect-only `factory/meta/asks.md` stub,
    and removed the duplicate meta artifact entries from the code-owned
    artifact contract while adding a duplicate-path regression guard
- the canonical checked-in customer-ask backlog is active and centralized:
  - `factory/logs/meta/asks.md` is the canonical checked-in backlog
  - `factory/meta/asks.md` is now a checked-in redirect-only compatibility stub
  - the live ask categories currently include `release plans`,
    `system deficits`, and `quality`
  - no ask is marked urgent
- the checked-in root artifact contract is currently consistent with the docs:
  - `internal/testpath/artifact_contract.go` now carries one entry per
    classified root artifact path
  - `pkg/testutil/artifact_contract_test.go` now fails fast if duplicate
    normalized paths re-enter the code-owned contract list
  - `docs/development/root-factory-artifact-contract-inventory.md` now includes
    the redirect-only `factory/meta/asks.md` stub and no longer describes the
    solved duplicate-entry defect as live
- the remaining cleanliness drift is in repository-local workflow input residue,
  not the checked-in contract surfaces:
  - `factory/inputs/idea/default/dedupe-root-factory-artifact-contract-entries.md`
    still describes a lane that landed in pull request `#14`
  - `factory/inputs/task/default/inventory-remaining-contract-guard-walkers.md`
    still points at a lane that landed in pull request `#8`
  - `factory/inputs/task/default/stabilize-root-factory-starter-contract.md`
    still points at a lane that landed in pull request `#11`
  - `factory/inputs/task/default/standardize-contract-guard-skip-policy.md`
    still points at an older contract-guard skip-policy lane that already
    landed in earlier merges

## current blockers

1. the checked-in meta world-state surfaces had drifted behind `main` and were
   still advertising the pre-`#14` world before this update.
2. the local workflow input inboxes still contain solved idea and task files,
   which can cause redispatch of already-landed cleanup lanes.
3. the current non-urgent customer asks remain broader than the repo needs
   while local workflow hygiene is still stale.

## theory of mind

- the repository is stable enough to avoid a broad stability audit.
- the highest-value work is still control-plane honesty and cleanliness, not a
  broad feature ask from the non-urgent backlog.
- the checked-in root-factory artifact contract, canonical ask backlog, and
  redirect-only legacy ask stub are now aligned on `main`.
- the remaining immediate risk is operational drift in repository-local inboxes:
  stale solved requests make the local factory surface look more active than it
  really is and can send workers back into closed lanes.
- the right customer rule remains:
  - `factory/logs/meta/asks.md` is the only live checked-in customer-ask
    backlog
  - the current asks are backlog inputs, not approved in-flight work
  - stability and world-model accuracy stay ahead of speculative product work
    unless an ask is marked urgent

## next best move

- do not start the release, CI/CD, throttle-guard, or website-quality asks yet.
- keep the checked-in meta surfaces current with `main`.
- dispatch one narrow cleanup idea that prunes solved repository-local workflow
  input residue under `factory/inputs/idea/default/` and
  `factory/inputs/task/default/`, leaving only active inputs or sentinels.
- after that inbox-hygiene lane lands, reassess whether the next move should be
  another narrow cleanliness pass or the highest-value non-urgent customer ask.

## customer asks

- `factory/logs/meta/asks.md` currently carries active asks under `release
  plans`, `system deficits`, and `quality`.
- no explicit urgency marker or top-ranked ask is recorded in the checked-in
  backlog.
