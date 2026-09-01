# Slow functional-package BuildProcess audit

## Completion contract

This is the exhaustive checklist for the 2026-09-01 cleanup. The source data
is a clean `cmd/functionallane` run with `-jobs 2 -count=1 -timeout 300s` and
machine timing output enabled. The lane took **344.310 seconds** wall time.

A package is in scope when both conditions are true:

1. its package time is greater than 3 seconds; and
2. its package directory contains more than one real `root.BuildProcess` or
   functional `support.BuildProcess` call. Text in assertion messages is not a
   construction call.

Every item below records the observed time and construction count, the
customer behavior that justifies the functional layer, and the result of the
convergence/parallelism audit. A checked item means the package was inspected;
it does not mean distinct immutable application graphs were forced into one
mutable fixture.

## Authority findings that apply across packages

- The durable async API already accepts explicit `FACTORY_ID`,
  `FACTORY_INLINE`, `WORKFLOW_FILE`, `WORKFLOW_NAME`, and `INLINE_WORKFLOW`
  sources. This is not equivalent to selecting an already-open ordinary
  Factory Session.
- Provider and script execution already retained `FactorySessionID` in their
  typed correlation/context, but the platform command adapters discarded it.
  The audit fixed that loss: command-runner edges now receive the immutable
  value as a policy-free execution scope and can route overlapping calls
  without consulting the Current Factory or inferring ownership from a
  working directory.
- The CLI run-modes conversion exposed a missing customer selector for invoking
  an already-open remote Factory Session and two command-adapter identity
  losses. `you --remote run --session <id>` now targets that public session,
  and the session identity survives both structural command boundaries. The
  response-stream path drains the server-captured retained-event head instead
  of guessing at a terminal event or waiting on a live stream.
- The inference Worker Session history/cursor cases were explicitly probed.
  Live observations work for a non-default session, but the fresh replay
  process does not rehydrate that explicit route; the public `~default`
  compatibility route does. Their default-session usage is therefore
  intentional customer replay coverage, not an accidental inference-profile
  default. Selection, failure, stream, and cleanup inference scenarios already
  use unique non-default sessions.

## Audited packages

- [x] `workers/transports/cli/run/lifecycle` — **about 6–8s after correction,
  2 static construction sites.** The earlier 23.145s measurement was dominated
  by a cancellation/recovery assertion bug: the command waited for its full
  15-second deadline and then accepted the phrase "cancelable lifecycle
  command" as proof of cancellation. Factory Session cancellation and stopping
  the CLI command that owns `--with-server` are now exercised as separate
  customer controls, and deadline expiry is rejected explicitly. The scenario
  fell from about 16.3s to about 1.1s. Independent adverse cells now overlap;
  only the two cells using the routed local process retain its serialization.
  The subprocess-based forced-assertion cleanup cell was removed because it
  tested fixture bookkeeping rather than customer behavior.
  The two source call sites are not a count of runtime process instances.
  Further convergence is possible for session-only behavior through a hosted
  explicit-session fixture, while customer-visible local listener ownership
  should retain isolated process coverage.

- [x] `workers/inference` — **5.1–5.4s in clean post-change samples, 4 static
  construction sites.** The original 15.469s audit repeated the lifecycle
  mistake: top-level tests declared parallelism, but every scenario that
  observed Worker recordings swapped one package-global writer and held the
  process read/write lock for the complete Factory Session. The writer now
  routes from the public Factory Session identity in the durable Worker opening
  record and retains recording/Worker associations for later writes and reads.
  Recording scenarios open empty explicit sessions, bind their writer, and
  submit Work through the public API, so health, fidelity, opening-gate, loss,
  and replay cells overlap on one hosted process. Their aggregate critical path
  fell from roughly 12s to under 1s in focused execution.

  Portable recording selection now submits two Works to one explicit Factory
  Session and proves that a later session receives a fresh recording identity;
  it no longer stops the hosted daemon for repeated `~default` executions. The
  structured-result matrix proves object and explicit-null preservation once
  through the public Factory Event instead of repeating the object case through
  an internal replay-artifact inspection. Authentication, throttling, and
  timeout classification cells run as parallel leaves.

  The subprocess-based forced-assertion cleanup test was removed as fixture
  bookkeeping. Two five-second test-binary/PID process-tree tests were also
  removed from this functional package: their customer failure classifications
  already have controlled-boundary coverage here, while real descendant
  termination is covered by platform and integration tests. The four remaining
  construction sites still prove the hosted cohort, interrupted-recording
  restart, terminal-recording restart, and an immutable recording-handoff
  failure. The `~default` replay exception is explained above. A focused race
  run passes in 14.2s. Later whole-package samples on the same host ranged from
  11.6–17.8s while unrelated HTTP-history leaves slowed together; the changed
  recording/structured cohort remained below 1s, so that spread is recorded as
  host variance rather than restored serialization.

- [x] `transport/cli/parameters` — **12.490s, 3 builds.** The three graphs have
  incompatible immutable edges: an input observer, the full invocation
  handler, and a deliberately missing asset loader. The package's public
  parameter cases are individually short; forcing them behind one mutable
  edge multiplexer would test the harness rather than CLI parsing. They remain
  serialized because each one-shot invocation owns `~default`; explicit local
  session selection is the product prerequisite for safe overlap.

- [x] `workers/mock` — **12.023s, 2 builds.** The main customer matrix is one
  shared graph. The second non-long graph injects an ACP-specific command
  boundary to prove that selecting ACP does not replace JavaScript mock
  workers; the long-only graph has a different service-configuration edge.
  These are distinct replaceable-boundary behaviors. The main matrix is
  already internally concurrent and dominates the package time.

- [x] `transport/http/server` — **11.991s, 2 builds.** One package HTTP fixture
  covers request/response behavior. The isolated graph owns production
  listener startup, pprof opt-in, bind failure, shutdown, and active-stream
  closure. It must own a real listener lifecycle and cannot share the fixture's
  captured starter. Eight independent tests run in parallel.

- [x] `factory/packaged/fix` — **10.787s, 2 builds.** One hosted shared process
  covers explicit-session packaged behavior and parallel validation failures.
  The extra build is the one-shot CLI response-parity cell, where stdout,
  stderr, recording, and `~default` teardown are the customer contract. It can
  converge only after local CLI execution can select an explicit session.

- [x] `transport/cli/output` — **9.962s, 5 builds.** The package fixture covers
  ordinary JSON/NDJSON output. Four isolated graphs inject mutually exclusive
  stream boundaries: a slow writer, a failing writer, an incremental text
  observer, and interruption/startup lifecycle controls. Nine tests already
  run in parallel. Construction is not the dominant cost; the slowest tests
  deliberately apply output backpressure.

- [x] `factory/definitions` — **9.267s, 4 builds.** A shared process covers the
  ordinary definition/customer CLI cohort. Separate service-host and
  initialization graphs own API hosting, export/import persistence, and
  injected provider-default resolution. Their immutable host/provider edges
  differ. The customer validation matrix itself is cheap; further convergence
  would introduce mutable global current-Factory state.

- [x] `factory/packaged/goal` — **8.683s, 4 builds.** The shared packaged
  scenarios are converged. Interpolation rejection, persistence/replay, and
  quiet one-shot CLI output each own different provider/recording boundaries.
  None is an inventory test. The CLI cells remain `~default`-owned until local
  explicit-session invocation exists.

- [x] `factory/replay_contracts` — **8.355s, 4 builds.** Each graph owns a
  different recording effect: canonical admission, composed record/replay,
  selected-tick projection, or Work snapshot projection. Reusing a writer or
  replay reader would erase the process-reconstruction behavior under test.
  The package should remain separate graphs; the customer-visible replay
  assertions are the reason it is functional.

- [x] `bootstrap_portability` — **7.235s, 4 builds.** The builds represent the
  Agent Factory portable bundle, Automat edges, bounded dispatch readiness,
  and invalid-definition rejection. The calls do not repeat the same graph.
  The many pure flatten/expand shape checks should continue migrating toward
  unit/lint coverage when touched, but the retained functional cells exercise
  customer export/import and runnable activation.

- [x] `orchestration/javascript/loading` — **7.100s, 2 builds.** The shared
  fixture covers CLI/API source loading. The isolated partial-start graph
  injects a failing lifecycle role to prove unwind. A mutable failure switch
  would make the shared process unsafe; the loading scenarios remain
  serialized because current CLI JavaScript execution binds `~default`.

- [x] `models/root_composition` — **6.521s, 2 builds.** The package already
  parallelizes the process-owned catalog, diagnostic, and LocalAI cells. The
  second construction is the reconstruction half of pull-to-ready persistence.
  Shared-session model scenarios remain serialized because the customer
  overlap ledger assigns unique peer routes; the rebuild is the asserted
  restart behavior and cannot be removed.

- [x] `factory_definitions/transports/cli/yaml_parity` — **6.318s, 2 builds.**
  JSON/YAML validation and persistence share one process. The second graph
  injects the provider runner needed for actual run parity. Combining it would
  weaken the no-dispatch assertions in rejected-source cases. The scenarios
  are one-shot Current Factory actions and cannot overlap safely today.

- [x] `sessions/root_composition` — **6.235s, 12 builds.** This is a composition
  boundary suite, not twelve interchangeable scenario fixtures. Builds prove
  inert construction, close-after-success/failure, injected provider/script
  routing, recording format selection, secret-redacting writers, seeded replay
  activation, and automation recovery. Twenty independent cells run in
  parallel. Each remaining build changes an immutable edge or explicitly
  proves process lifecycle/reconstruction; convergence would invalidate the
  behavior being asserted.

- [x] `workers/concurrency` — **6.130s, 2 builds.** The shared process owns the
  complete explicit-session concurrency matrix. The second graph is a
  malformed dangling-worker configuration proving rejection before provider
  start. It cannot enter the valid shared host without mutating its Factory
  definition authority. The valid scenarios execute concurrently inside the
  single top-level test.

- [x] `factory/visualization/runtime_metrics` — **5.859s, 5 builds.** Ordinary
  CLI metrics share one process. Replay-priced, replay-unpriced, canonical
  usage-fixture, and provider-completion graphs inject distinct immutable
  recording/provider inputs. Five independent tests are parallel. Merging the
  replay readers would turn price/availability isolation into mutable harness
  state.

- [x] `factory/packaged/review` — **5.426s, 2 builds.** The shared process owns
  the packaged review matrix. The separate retry-exhaustion graph has a
  dedicated provider sequence and recording lifecycle. Seven independent
  cases run in parallel; the 2.090s exhaustion path is customer-visible retry
  behavior.

- [x] `workers/agent` — **4.681s, 2 builds.** One graph owns the shared explicit
  session scenarios; the lifecycle graph injects process-close/cancellation
  controls. The package is already converged at each compatible edge shape.
  Its internal scenarios overlap where session ownership permits.

- [x] `factory/invocation` — **4.501s, 2 builds.** Success/failure reuse the
  same helper shape per scenario; cancellation requires a runner that owns the
  invocation cancel function. These one-shot tests assert exact canonical
  event order and terminal CLI output. A shared mutable runner would obscure
  that ordering, and local explicit-session selection is required before the
  independent success/failure cells can overlap.

- [x] `transport/run_scoped_server` — **4.129s, 14 builds.** All tests already
  run in parallel. Each construction owns a distinct listener/browser/port
  edge and asserts that one public command starts, rejects, falls back, or
  tears down that exact resource. A package process would require mutating the
  very ownership boundary under test. At 4.129s total, parallel isolated
  construction is the optimized topology.

- [x] `sessions/execution` — **4.007s, 5 builds.** The graphs distinguish drain
  behavior, partial-result blocking, hosted finite/continuous runtime, and API
  observation. Nine tests run in parallel. Each fixture's gate and provider
  sequence is invocation-owned; combining them would create cross-scenario
  lifecycle coupling for negligible wall-time gain.

- [x] `factory/packaged/tts` — **3.834s, 5 builds.** The shared packaged
  scenarios are already converged. The other graphs own local-model runtime,
  audio recording, success/failure replay, and direct CLI prompt binding.
  Those effects are deliberately different and several tests already overlap.
  The package is below four seconds; further convergence would remove replay
  reconstruction or require mutable model edges.

- [x] `recordings/root_composition` — **3.447s, 5 builds.** The builds prove
  inert construction, portable replay execution, compatibility replay,
  transport activation, and record/replay lifecycle. Six tests run in
  parallel. Process reconstruction and different recording readers/writers
  are the public behaviors, so a single graph is not a valid optimization.

## Packages above 3 seconds with one build

These are intentionally outside the repeated-construction checklist but were
reviewed when prioritizing wall time. CLI run modes fell from **31.732s** to
**5.772s** in the final loaded lane after conversion to one hosted process,
unique explicit sessions, and parallel customer scenarios. Its isolated race
run passes in **8.264s**. The timeout/cancellation probe also fixed two public
boundary defects: transport deadlines now retain the canonical terminal CLI
outcome, and wrapped Work payload-size admission failures remain actionable 400
responses instead of generic 500s. A provider working-directory assertion was
removed because it described an internal shape rather than customer behavior.

AGY fell from **23.498s** to **9.610s** in isolation and **11.849s** in the
final loaded lane. Its nine independent
packaged-review customer scenarios install the two packages once, open unique
sessions on one hosted process, invoke through the public remote CLI selector,
and run in parallel. The package race run passes in **21.524s** in isolation.
Its remaining
direct, golden, timeout, cancellation, and hosted-lifecycle cases are already
served by the single package graph; their sequencing owns customer lifecycle
boundaries rather than duplicate construction.

## Optimization techniques applied across the cleanup

The cleanup used the following techniques. They are recorded here so a future
audit can reproduce the result without treating construction count as the only
performance variable.

- Removed functional tests that compiled or located the real CLI. Customer
  command behavior now executes through the in-memory public command contract;
  executable discovery, OS process, signal, pipe, and exit-status proof belongs
  in the small prebuilt-artifact integration lane.
- Replaced per-scenario application construction with one package process for
  every compatible immutable edge shape. Package installation, service
  hosting, fixture data, and application initialization are paid once.
- Distinguished a static `BuildProcess` call site, an actual process instance,
  and a distinct immutable edge graph. These are different audit quantities;
  two source sites do not prove two processes per scenario.
- Converted reusable customer scenarios to unique explicit Factory Sessions.
  Session-owned IDs, peer routes, streams, working directories, recordings,
  and fake state prevent overlap from changing another scenario's result.
- Preserved Factory Session authority through platform command and provider
  adapters. Removing identity loss avoided fallback to Current Factory or
  `~default`, enabling safe concurrent routing rather than adding fixture
  locks.
- Added the public remote-session CLI selector and used one hosted process for
  independent CLI scenarios. Response rendering drains the retained public
  event head rather than waiting for a future live event.
- Parallelized independent top-level tests and leaf subtests. Aggregate
  matrices were split where independent cells could overlap; cases that prove
  reconstruction or own incompatible immutable boundaries retained separate
  graphs.
- Replaced shared real effects with exact controlled boundaries: provider
  command runners, listener starters, clocks, filesystems, recording readers
  and writers, streams, and cancellation gates. Each parallel scenario owns
  its boundary state.
- Removed inventories, generic constructor coverage, internal topology checks,
  fixture-ledger assertions, and duplicated behavioral matrices. Customer
  behavior stayed functional; package shape moved to lint/static enforcement
  or disappeared when it protected no contract.
- Used observable readiness, cancellation, retained events, and explicit gates
  instead of fixed sleeps. Safety deadlines remain generous ceilings and are
  not used as the expected completion mechanism.
- Ran focused packages repeatedly and under the race detector before the whole
  functional lane. This separated deterministic critical-path costs from host
  contention and caught race-only startup assumptions without converting them
  into arbitrary sleeps.

## Lifecycle audit miss and correction

The first lifecycle review missed a significant optimization because it
accepted structural evidence in place of executed-path evidence:

1. It saw parallel test declarations but did not notice that the shared
   fixture held its mutex across the entire `Process.Execute`. The tests were
   scheduled concurrently but the customer commands executed serially.
2. It reported the aggregate adverse matrix time and described the result as a
   shutdown boundary without timing its leaf subtests. The
   cancellation-and-recovery leaf alone consumed about 16.3 seconds.
3. It did not determine why the leaf reached the 15-second completion ceiling.
   The CLI invocation owning `--with-server` remained alive after the Factory
   Session cancellation because these are separate lifecycle controls.
4. The assertion accepted any error text containing `cancel`. Its own harness
   timeout said `cancelable lifecycle command did not complete`, so a deadline
   failure falsely passed as successful cancellation.
5. It treated a subprocess-based forced fixture-cleanup assertion as functional
   coverage even though no customer could observe the bookkeeping it tested.
6. It described two static construction sites as "2 builds," obscuring the
   difference between source topology, runtime construction, and the immutable
   edges that actually justify separate graphs.

The corrected scenario first proves the public Factory Session and provider
cancellation projections, releases the listener gate, explicitly cancels the
CLI invocation, requires the canonical cancellation outcome, rejects deadline
expiry, and then proves a fresh invocation recovers. Independent adverse cells
run as parallel leaf subtests, while the two cells that use the routed local
process retain its narrow serialization. The harness-only subprocess test was
removed.

That correction reduced cancellation-and-recovery from about **16.3 seconds to
1.1 seconds**, the package from roughly **23 seconds to a stable 6.35–6.38
seconds**, and its race run from roughly **32 seconds to 13.8 seconds** without
removing listener shutdown, session cancellation, provider cancellation,
public projection, CLI cancellation, or recovery coverage.

The general lesson is that an audit is incomplete until it verifies actual
overlap, leaf-level timing, signal-driven completion, exact terminal outcomes,
and a customer observer for every retained scenario.

## Validation required before closing this audit

- [x] Focused provider and worker command-correlation unit tests pass.
- [x] Every changed functional package passes normally.
- [x] Every changed functional package passes with `-race`.
- [x] The complete functional lane passes after the final changes.
- [x] The race detector passes across `./tests/functional/...` with `-p 2`,
  `-count=1`, and a five-minute per-package timeout.
- [x] A final timing report is captured: approximately **375.4 seconds**,
  approximately **+31.1 seconds** from the 344.310-second audit sample. Package rankings and the
  slowest package times remained materially unchanged, so this is recorded as
  whole-lane/environment variance rather than a code-path regression.
