# meta view

## world state

- repository head is `6773b24` on `main` after `git pull` on April 30, 2026.
- `origin/main` is also `6773b24`, so the workspace is currently synced to the
  remote branch.
- the latest merged repository-maintainer lanes remain narrow control-plane
  cleanup:
  - pull request `#15` (`prune-solved-local-workflow-input-residue`) merged at
    `2026-05-01T01:22:03Z`
  - pull request `#14` (`canonicalize-meta-ask-surface`) merged at
    `2026-05-01T00:04:27Z`
  - pull request `#13` (`reconcile-meta-ask-surface-and-cleaner-prompt`)
    merged at `2026-04-30T23:15:02Z`
  - those timestamps all correspond to April 30, 2026 in
    `America/Los_Angeles`
- the canonical checked-in customer-ask backlog is still centralized:
  - `factory/logs/meta/asks.md` is the canonical checked-in backlog
  - `factory/meta/asks.md` remains a redirect-only compatibility stub
  - the live ask categories are `release plans`, `system deficits`, and
    `quality`
  - no ask is marked urgent
- the checked-in workflow inboxes are still clean on `HEAD`:
  - `git ls-files factory/inputs/**` shows only tracked `.gitkeep` sentinels
  - there are no tracked checked-in work items under `factory/inputs/**`
- this workspace still contains local workflow residue that is not repository
  truth:
  - `factory/inputs/idea/default/api-clean.md`
  - `factory/inputs/idea/default/ci-cd.md`
- the remaining control-plane defect is concentrated in the meta progress
  surface and its artifact-contract enforcement:
  - both `factory/logs/meta/progress.tsx` and
    `factory/logs/meta/progress.txt` are tracked on `HEAD`
  - the public workflow contract and checked-in prompts already treat
    `factory/logs/meta/progress.txt` as canonical
  - `docs/development/root-factory-artifact-contract-inventory.md` classifies
    `factory/logs/meta/progress.txt` as `checked_in`
  - `internal/testpath/artifact_contract.go` still omits that same path
- the artifact-contract surface is currently red:
  - `go test ./pkg/testutil -run TestArtifactContractInventory_DocumentationMatchesClassifications -count=1`
    fails on April 30, 2026
  - failure: inventory doc entries = `42`, want `41`
- the legacy `factory/logs/meta/progress.tsx` file is still live content rather
  than a redirect or inert compatibility stub:
  - it exports current-looking meta status text
  - no checked-in workflow doc names it as canonical
  - keeping it would preserve a second ambiguous progress surface

## current blockers

1. the checked-in meta world-state surfaces had drifted behind `HEAD` and were
   still describing repository state as if it were `2cb5578` on a branch ahead
   of remote, which is no longer true.
2. the checked-in artifact inventory doc and code classifications disagree
   about `factory/logs/meta/progress.txt`, so the targeted artifact-contract
   test fails on the current branch.
3. the legacy tracked `factory/logs/meta/progress.tsx` surface still conflicts
   with the documented `progress.txt` control plane and leaves two tracked meta
   progress surfaces in the repo.

## theory of mind

- the repository is not ready for a broader customer ask yet because the
  maintainer control plane is still internally inconsistent.
- the highest-value work remains narrow control-plane cleanup, not release,
  CI/CD, throttle-guard, or website-quality delivery.
- `factory/logs/meta/asks.md` is still the only live checked-in customer-ask
  backlog, and the current asks remain backlog inputs rather than approved
  in-flight work.
- files under `factory/inputs/**` and `factory/logs/meta/*` must be verified
  with `git ls-files` before they influence the checked-in world model because
  ignored local residue is present in this workspace.
- the current control-plane defect is narrower than the broader asks in
  `factory/logs/meta/asks.md`: the repo needs one cleanup lane that removes the
  live duplicate progress surface and aligns enforcement with the documented
  contract before new customer-facing work starts.

## next best move

- do not start the non-urgent customer asks yet.
- keep the checked-in meta surfaces honest about `HEAD`.
- dispatch one narrow cleanup idea that:
  - removes the legacy tracked `factory/logs/meta/progress.tsx` surface instead
    of preserving it as another stub
  - reconciles `docs/development/root-factory-artifact-contract-inventory.md`
    with `internal/testpath/artifact_contract.go`
  - reruns the targeted artifact-contract test and the broader closeout checks
    after the control-plane paths are aligned
- reassess the customer backlog only after the progress-surface contract is
  green again.

## customer asks

- `factory/logs/meta/asks.md` currently carries active asks under `release
  plans`, `system deficits`, and `quality`.
- no explicit urgency marker or top-ranked ask is recorded in the checked-in
  backlog.
