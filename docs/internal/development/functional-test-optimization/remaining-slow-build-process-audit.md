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

- [x] `workers/transports/cli/run/lifecycle` — **23.145s, 2 builds.** The shared
  graph owns ordinary one-shot CLI cases. The second graph is the adversarial
  listener/process coordinator with invocation-scoped API starters and forced
  failure/cancellation ownership. Six independent customer tests are parallel;
  the 21.110s failure case is a real clean-invocation shutdown boundary. The
  graphs cannot be merged without making a listener starter mutable while a
  command is live.

- [x] `workers/inference` — **15.469s, 4 builds.** One hosted graph serves the
  normal explicit-session matrix in parallel. The remaining graphs prove
  interrupted-recording restart, terminal-recording restart, and a recording
  handoff gate whose writer is an immutable application edge. These are
  process reconstruction and edge-failure customer behaviors, not duplicate
  scenario fixtures. Twenty-eight parallel calls are present. The
  `~default` replay exception is explained above.

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
