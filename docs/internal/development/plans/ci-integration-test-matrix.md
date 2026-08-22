# CI Integration Tier — One Test Per Entrypoint, Installed Binary, Clean Directory

---
author: operator
last modified: 2026, august, 22
doc-id: PLAN-INT-001
status: proposed
---

# problem statement

We have no CI tier that runs the shipped binary the way a customer runs it, so an entire class of defect reaches users unopposed — including entrypoints that cannot work at all once installed, because they read files that only exist inside our repository.

## customer ask

One integration test per entrypoint of sufficient complexity, happy path only, kept
extremely small because the tier is expensive: ad-hoc invocation with mock workers across
a couple of packaged factories, JavaScript invocation, server mode, MCP mode, resume, and
replay. Validation may run, but pass/fail only.

## solution

Eight tests. Each spawns the compiled binary in a clean directory with no repository
reachable, drives one entrypoint end to end, and asserts an observable outcome that is not
the exit code. Every entrypoint the adversarial review found broken is represented, paired
with the system-level fix that has to land for its test to be passable at all.

# original document

The 33-agent and 41-agent adversarial evaluations of `you-agent-factory` v0.0.8, plus
`docs/internal/development/plans/next-work-meta-plan.md` for sequencing.

## Why a small tier, and why it still finds things

Three measured facts shape the design.

**1. The test already exists and does not run on pull requests.**
`tests/release/root_process_smoke_test.go::TestRootProcessCompiledBinaryModeMatrix`
already builds the binary, points `HOME`/`USERPROFILE` at a `t.TempDir()`, and runs
`run --dir <factory> --with-mock-workers`. But `./tests/release/...` is in neither
coverage lane — `cmd/gocoveragecheck/main.go:55` pins `unitTestPatterns` to `./pkg/...`
and line 56 pins the functional lane to `tests/functional`. The package appears only in
`development-package.yml` and `release-candidate.yml`, and there only two
`TestGoInstallSmoke_*` tests are invoked by name. The binary-mode matrix is never called
by any workflow.

**2. If it did run, it could not fail.** Its strongest assertion is
`if err != nil { t.Fatalf }`. Batch mode exits 0 regardless of work outcome
(`orchestration/runtime/factory.go:587` — a token routed to a failed place is not an
engine error). A happy-path suite built on exit codes alone would inherit exactly that
defect, which is why every test below asserts a **terminal state or a produced artifact**,
never only the exit status.

**3. It exercises no endpoint.** No HTTP surface, no `mcp serve`, no
`session list --scope persisted`. That is precisely where the installed-binary defects
live, and they are invisible to every tier we run.

**Why happy-path-only is the right trade.** Unhappy paths are cheap to test near the code
and expensive to test through a process boundary. Their value at this level is mostly
duplicated by the functional tier. What is *not* duplicated anywhere — and what no unit
test can reach — is whether the shipped artifact starts and completes a real journey with
no repository underneath it. That is the entire job of this tier, and it needs eight
tests, not sixty.

## Story 0 — Truthful exit codes (BLOCKING PREREQUISITE)

Batch `you run` must exit non-zero when a submitted work reaches a failed terminal state,
when a circuit breaker trips, or when a script worker exits non-zero. One-shot
`you run --named` already does this correctly and returns parseable detail, so the
semantics are settled; only the batch path needs aligning.

**A happy-path tier makes this more important, not less.** If exit 0 is unconditional,
a suite of eight happy-path tests is eight assertions that the process did not crash.
Story 0 is what converts them into assertions that the work succeeded.

Acceptance: a factory whose single work routes to a failed terminal state exits non-zero
from batch mode with the failed work name and terminal state on stdout; a factory whose
work completes exits 0; **a factory where every work item fails still exits non-zero** —
that last case is the one the evaluation measured returning 0. All asserted from a
compiled binary.

This is its own lane and merges on its own. Nothing else here is meaningful until it lands.

## The tier

Every test runs one process spawned from the compiled binary, with `HOME`/`USERPROFILE`
and the working directory both set to a fresh `t.TempDir()`, and **no repository reachable
by walking parents**. All eight are fixture-free by construction; the harness enforces it
(see "The no-internal-files rule").

| ID | Entrypoint | What it drives | Observable assertion (not the exit code) |
|---|---|---|---|
| **I1** | Ad-hoc invocation, packaged factory A | `run --named` against a shipped packaged factory with mock workers | Primary result present on stdout; work reaches a complete terminal state |
| **I2** | Ad-hoc invocation, packaged factory B | A second shipped packaged factory with a different topology (multi-workstation) | Every work reaches a complete terminal state; dispatch count matches the topology |
| **I3** | JavaScript invocation | A factory whose orchestration is a JS workflow, children served by mock workers | The workflow's JSON return value is on stdout and matches the expected shape |
| **I4** | Server mode | `serve`, then over HTTP: list sessions, list work, list workers | Bound URL printed; a submitted work is enumerable and reaches a complete terminal state through the API |
| **I5** | MCP mode | `mcp serve`, handshake, tool listing, one tool call | Handshake completes and the expected tool set is returned, from a temp directory |
| **I6** | Resume | Start a run, stop the process, resume it | The resumed run continues from recorded progress and reaches a complete terminal state |
| **I7** | Replay | Record a run, replay it | Replay reproduces the recorded events for the unchanged input |
| **I8** | Validation | `config validate` over a small corpus | **Pass/fail verdict only** — every valid factory passes, every invalid one fails. No assertion on diagnostic text |

Notes that are part of the contract, not commentary:

- **I1 and I2 use real shipped packaged factories**, not bespoke test fixtures. A fixture
  we author for this tier proves the harness works; a shipped factory proves the product
  does. Two is the right number — one is an existence proof, three is a matrix.
- **I8 asserts the verdict, never the message.** Diagnostic wording is owned by the unit
  tier. Asserting it here buys nothing and makes the tier brittle to correct changes.
- **No unhappy-path rows.** Mock-worker failure modes, selector no-match, breaker trips,
  pagination edges, flag rejection, port collisions and error-code classification are all
  owned elsewhere — `PLAN-MOCK-001`, `PLAN-EXT-001` EXT-2, and the functional tier. If one
  of them regresses, this tier is not where it should be caught.

## Adverse-review failures this tier owns

These are the entrypoint-level failures from the adversarial review. Each is paired with
the **system-level fix** required for its test to be passable, because in every case the
test cannot be written first — the entrypoint does not work at all.

| Finding | Status | Covered by | System-level fix required |
|---|---|---|---|
| MCP surface unreachable from an installed binary — `execution_contract.go:28` hardcodes `ContractFixtureCatalogRelativePath` to `pkg/transports/http/testdata/durable-session-contract-fixtures.json`, and `executionopening/factory.go:285-308` walks parent dirs for it, failing `fixture catalog not found; run from the repository root` | **verified here** | I5 | The runtime must not read a repository test file. Embed the catalog, derive it from the contract, or remove the dependency — but the released binary must carry everything it needs |
| `session list --scope persisted` / `session dispatches` unusable when installed | verified here (same root cause) | I4 | Same fix as above |
| Batch `you run` exits 0 when every work item FAILED; no machine-readable failure signal at any verbosity including `--json` | reported | Story 0, then all of I1–I7 | Story 0 |
| Resume does not survive a hard kill — durable sessions are not flushed and stay at `RUNNING` / `FinishedAt: null` after a clean exit | verified here | I6 | `PLAN-REC-001` — resume reads the recording, which is flushed every 250 ms, rather than the durable snapshot, which is not |
| Replay serves stale output after an input change; drift is detected only as a WARN on stderr | reported | I7 | `PLAN-EXT-001` EXT-5 — drift is an error by default, with an explicit opt-in, and the tolerance is recorded in the artifact |
| Canonical JavaScript examples use bare top-level `await`, which the runtime rejects; the required wrapper is documented nowhere | reported | I3 | `PLAN-DOC-001` — the JS example becomes a real transcluded directory, which I3 then runs |
| Two of nine `agent.run` fields absent from the shipped contract artifact | verified here | I3 (indirectly — the workflow exercises the fields) | `PLAN-JS-001` JS-P7 / `PLAN-DOC-001` DOC-4 |
| Dead entries in the shipped factory catalog | reported | I1, I2 | Whichever packaged factories I1/I2 select must load; a dead catalog entry is a delivery defect, not a test-fixture problem |

The pattern across the first two rows is the one worth naming: **an entrypoint that only
works inside our repository is not a tested entrypoint, it is an untested one that
happened to pass.** Every one of our existing tiers runs with the repository as the
working directory, so none of them can see this class at all.

## The no-internal-files rule

The mechanism matters more than the list above, because the list is only what two
evaluations happened to find.

The harness asserts that a test process **reads nothing under the source tree**. Any
access to a repository path from a binary running in an isolated `HOME` and working
directory fails the test, naming the path. That turns "does this entrypoint smuggle a
repo dependency" from a thing we discover in a customer report into a thing that fails in
CI on the PR that introduces it.

This single rule is the highest-value line in the plan. It is what would have caught the
fixture catalog years earlier, and it is what will catch the next one without anyone
having to think of it.

## Delivery steps

Each merges on its own and leaves `main` releasable.

1. **Story 0 — truthful batch exit codes.** Blocking prerequisite.
2. **Harness package `tests/integration/`** — builds the binary once per run, spawns it in
   an isolated `HOME` and working directory, and enforces the no-internal-files rule.
   Ships with **I8 only**, because validation needs no server and proves the harness
   cheaply.
3. **Wire the job into `ci.yml`** as `Backend Integration`, required. Adding it while it
   holds only I8 means the gate exists before it has teeth, so a later red is
   unambiguously a product regression rather than a new gate.
4. **I1, I2, I3** — the invocation entrypoints. Depend on Story 0.
5. **I4** — server mode.
6. **I5** — MCP mode. Cannot pass until the fixture-catalog dependency is removed; that
   removal is its own lane and this test is its acceptance evidence.
7. **I6, I7** — resume and replay. Sequenced last: I6 depends on `PLAN-REC-001` REC-6, and
   I7 depends on `PLAN-EXT-001` EXT-5.

Steps 4 through 7 are independent of one another and can run in parallel once Story 0 and
the harness are in.

## Non-goals

- **No real provider calls.** Every test runs against mock workers or stub binaries.
- **No unhappy-path coverage.** Stated above and repeated here because it is the
  constraint most likely to erode: the first person to add "just one" failure row should
  be pointed at this line.
- **No replacement of the functional tier.** This tier answers "does the shipped artifact
  work", not "is this package correct".
- **No growth without removal.** The tier has a budget; a ninth test means a case has been
  made that an existing one is redundant.
- **No new assertions inside `tests/release/`.** That package keeps its release-artifact
  role; the binary-mode matrix moves here rather than being duplicated.

## Verification and budget

Target wall time **under two minutes** on a four-core hosted runner. Eight tests, one
shared binary build, all independent and run in parallel. If the tier exceeds budget, the
answer is to remove a test, not to move it off the required path — an expensive tier that
is not required is a tier that does not exist.

## Delivery loop

Implementation finishes when its final head is pushed, the PR is open, CI has started,
and blocking review feedback is addressed. Review owns terminal-and-passing CI, conflict
resolution, and merge. CI-run evidence goes in a PR comment, never a commit.
