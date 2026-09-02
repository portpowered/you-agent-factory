# Slow functional-package BuildProcess audit

## Completion contract

This is the exhaustive checklist for the 2026-09-01 cleanup. The source data
is a clean `cmd/functionallane` run with `-jobs 2 -count=1 -timeout 300s` and
machine timing output enabled. The lane took **344.310 seconds** wall time.
After the blocking/non-blocking corrections below, the same controlled
`go test -p 2 ./tests/functional/... -count=1 -timeout=5m` lane passed again in
approximately **296 seconds**, down from the immediately preceding **327.303-
second** run. Loaded package times included **16.742s** for Chat Session ACP
composition, **12.704s** for `providers/agy`, **9.772s** for
`sessions/restart`, **7.024s** for Factory Builder, **3.471s** for CLI output,
and **1.447s** for the packaged catalog. No lifecycle or profile-contention
failure occurred.

After the subsequent blocking-span and customer-boundary cleanup, the same
controlled lane passed in approximately **249 seconds**. Current loaded package
times include **16.641s** for Chat Session ACP composition, **9.531s** for
`sessions/restart`, **8.469s** for named invocation, **8.116s** for Factory
definitions, **6.765s** for Workers/mock, and **4.411s** for `providers/agy`.

After the final blocking/non-blocking pass, the canonical
`cmd/functionallane -jobs 2 -count=1 -short=false -timeout 20m` lane passed end
to end. Representative loaded package times were **5.949s** for CLI commands,
**4.679s** for Workers/mock, **3.128s** for CLI parameters, **2.119s** for the
HTTP server, and **5.900s** for packaged Goal. The complete bounded race lane,
`go test -race -p=2 ./tests/functional/... -count=1 -timeout=20m`, also passed
with no races. A raw unbounded `go test ./tests/functional/...` run is not a
valid comparison: its package fan-out starved concurrent application startup
and packaged-installation ownership, producing failures absent from both
canonical bounded lanes. The testing standard now requires the canonical job
budget for broad verification.

After the 2026-09-02 standards and ownership-phase follow-up, the same
canonical normal lane passed in approximately **260 seconds**. All normal
package times were below 10 seconds except the explicitly deferred Chat
Session ACP composition package at **24.735s**. Representative changed package
times were **3.467s** for CLI parameters, **4.273s** for Workers/mock,
**2.364s** for HTTP server, **8.121s** for JavaScript loading, **6.893s** for
Models root composition, **7.789s** for Petri dispatch, and **6.267s** for
packaged Subagent. The complete bounded race lane then passed with
`go test -race -p=2 ./tests/functional/... -count=1 -timeout=20m`; no race or
deadline failure was reported. The remaining Chat Session work stays in
`docs/internal/plans/default-session-authority-for-concurrent-invocations.md`
because it requires a production ownership contract change rather than another
test-only scheduling change.

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

- [x] `workers/transports/cli/run/lifecycle` — **20.795s across five current
  runs (4.159s/run); three race runs took 29.462s (9.821s/run),
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
  The final blocking-span pass removed a fixed two-second listener-helper test
  that exercised the fixture rather than customer behavior. Invalid effort,
  conflicting prompt sources, and unreadable prompt paths are proven to fail
  before runtime activation, so they now bind only their immutable command
  route and execute without acquiring the invocation-wide runtime lease.
  Successful and runtime-owning local commands retain that lease. Further
  convergence is possible for session-only behavior through a hosted explicit-
  session fixture, while customer-visible local listener ownership should
  retain isolated process coverage.

- [x] `provider_sessions/cli` — **22.511s across five current runs
  (4.502s/run); three race runs took 18.523s (6.174s/run).** The package now
  owns one hosted process. Its test-binary recursion cell, forced assertion,
  cleanup report, constructor counts, listener census, and package-wide route
  assertions were fixture verification rather than customer behavior and were
  removed. The deadline/cancellation cells drove only the package-local
  readiness waiter and were removed; retained readiness cases observe the
  public selected target and malformed-definition rejection. Independent
  replay, readiness, and abrupt-stream customer cases now overlap with unique
  sessions and routes. The two fleet-wide journeys remain serialized because
  they intentionally observe the global Worker Session fleet; running them
  beside peer sessions changes the customer result rather than merely the
  harness. Parallel probing also exposed an “empty explicit-session list” cell
  whose CLI request actually selected the fleet endpoint when no Work ID was
  supplied. That misleading case was removed rather than preserving a passing
  assertion that depended on package quiescence.

- [x] `workers/inference` — **5.15–6.00s Go package time across five independent
  current runs (5.78s median), 5 static construction sites.** End-to-end command
  time for the three explicitly measured samples was 6.67–7.67s (7.64s median).
  Historical samples from
  the earlier topology ranged from 5.1–17.8s as unrelated HTTP-history leaves
  varied with host load; those slower valid samples remain part of the record
  rather than being hidden behind the best unloaded result. The original
  15.469s audit repeated the lifecycle
  mistake: top-level tests declared parallelism, but every scenario that
  observed Worker recordings swapped one package-global writer and held the
  process read/write lock for the complete Factory Session. The recording edge
  now carries Factory Session identity explicitly, so the writer routes without
  decoding an internal opening payload and retains recording/Worker associations
  for later writes and reads.
  Recording scenarios open empty explicit sessions, bind their writer, and
  submit Work through the public API, so health, fidelity, opening-gate, loss,
  and replay cells overlap on one hosted process. Their aggregate critical path
  fell from roughly 12s to under 1s in focused execution.

  Portable recording selection now submits two Works to one explicit Factory
  Session behind a deterministic admission gate. One focused, independently
  built one-shot process retains the distinct customer guarantee that reusing a
  `--record` path starts a fresh recording and Worker Session instead of merging
  prior history. Structured-result object and explicit-null behavior remains a
  live public Factory Event proof here; persisted object behavior moved to the
  existing `factory/replay_contracts` public record/replay journey, so it is
  observed after replay without adding another inference graph. Authentication,
  throttling, and timeout classification cells run as parallel leaves.

  Test-binary/PID process-tree cells, including the `functionallong` companion
  case, were removed from the functional package. Their customer failure
  classification remains covered through the deterministic command lifecycle
  edge, while real pipe and descendant termination remain in platform and
  integration tests. The Claude child-test forced-assertion cleanup cell was
  also removed because it proved fixture bookkeeping rather than customer
  behavior. The five construction sites prove the hosted cohort, the focused
  same-path one-shot contract, interrupted-recording execution, terminal
  recording execution, and restart/handoff behavior with incompatible immutable
  recording edges. The `~default` exception is limited to the customer-visible
  path-reuse and replay contracts. Two current focused race runs pass in
  13.17–13.94s; the public replay-contract race proof passes in 5.58s. The complete
  `workers/inference/...` race tree also passes, with the provider subpackages
  completing in 4.24–5.18s.

  A later blocking-span pass removed the functional package's command-router
  fixture test and invalid Workers-wire construction test because neither had
  a customer observer. Provider-selection integrations now bind only their
  fixed provider identity and clean up that identity, so distinct selections
  no longer acquire the package-wide exclusive process lock. Mutable provider-
  service overrides and non-session recording writers retain exclusivity.
  Three current full repeats took 16.161s total (5.387s/run), and three race
  repeats took 36.232s (12.077s/run). One preceding five-repeat attempt exposed
  a Worker-ID/provider-reference parity mismatch; the leaf then passed ten
  focused repeats, followed by the clean full and race samples above. That
  observed variance remains recorded for the broad lane rather than being
  presented as a proven deterministic fix.

- [x] `transport/cli/parameters` — **13.735s across five current runs
  (2.747s/run); three focused race runs took 17.676s (5.892s/run).** The three graphs still have incompatible
  immutable edges: an input observer, the full invocation handler, and a
  deliberately missing asset loader. The audit found that separate one-shot
  success tests repeated the same Unicode positional, embedded `=`, repeated
  key, nested JSON, and null/empty JSON behavior already proved by the combined
  reusable-process journey. Those duplicate cells were removed; malformed and
  invalid-JSON customer failures remain. The retained one-shot invocations own
  `~default`, so explicit local session authority remains the prerequisite for
  overlap. Pre-dispatch failures and remote optional-session parsing now
  overlap. The combined success witness no longer inspects the internal
  canonical submission record: its authored workstation prompt proves the
  expanded positional, named, URL, JSON-null, JSON-empty, object, and array
  values at the controlled provider boundary, while the overall successful
  invocation proves repeated values are accepted.
  The follow-up customer-boundary audit removed the detached input-observer
  graph entirely. Its parser inventory, retired-command duplicate, and
  flag-after-positional duplicate did not execute a customer handler and were
  already protected by focused public command tests. Unknown-flag diagnostics
  now use the normal reusable process. The package therefore owns two graphs:
  the customer application and the deliberately missing-asset application.
  A current verbose run took 3.564s; the focused race sample passed.

- [x] `factory/packaged/catalog` — **10.508s in the refreshed loaded lane;
  the required-input inventory alone took 8.72s loaded and 6.81s focused.
  Five optimized runs took 4.759s total (0.952s/run); focused race 5.177s.**
  The slow test dynamically discovered every packaged Factory with a required
  parameter and repeated the same pre-provider missing-input behavior for each
  entry. It now retains one representative `@you/fix` customer invocation that
  proves the typed rejection, absent success result, zero provider calls, and
  actionable Factory/parameter diagnostic. Exact embedded/runtime parity,
  unique-name/source shape, matrix registration, exact catalog count, every-
  entry metadata, ACP parameter-shape, historical two-Factory validation, and
  forced-unwind reports were inventories or harness assertions and were
  removed from the functional lane. The public API still proves a non-empty
  catalog with representative discoverable entries, while customer override,
  atomic failure, cancellation, and non-mutating list behavior remain.

- [x] `workers/mock` — **19.302s across five current runs (3.860s/run),
  three focused race runs took 30.187s (10.062s/run).** The main customer matrix is
  one shared graph. The second non-long graph injects an ACP-specific command
  boundary to prove that selecting ACP does not replace JavaScript mock
  workers. Service-configuration alignment cells were removed because they
  asserted internal WorkDir/environment plumbing, not customer behavior.
  Duplicate output-format variants were removed where the same customer
  success/failure was already protected in the output package. The slow mock
  failure now uses one public retry attempt and an observable dispatch gate
  instead of repeating the default retry policy; ten focused repetitions took
  7.770s total. Independent explicit-session cells overlap; the remaining
  local one-shot cohort is serialized by `~default` ownership. The plain-batch
  empty, already-terminal, and continuous-idle counterexample matrix was also
  removed here: the same customer behaviors are already proved in
  `sessions/execution`, and this copy added a fixed 500ms negative-liveness
  observation with no public completion signal.
  A direct hosted-session probe confirmed that public remote `run` rejects
  batch `--work` input with `REMOTE_INVOCATION_INPUT_REQUIRED`. Replacing these
  rows with `submit batch` would test a different customer report, so the
  remaining local batch cohort is retained; the separate ACP-selection graph
  now overlaps the shared matrix at package level. The standalone default
  success row was removed because the all-failed journey already proves a
  clean success after recovery on the same customer boundary. The named human
  failure row was also removed here because the CLI output suite owns a more
  complete public failure-rendering contract; retaining it in Workers only
  repeated a serialized `~default` invocation.
  The follow-up removed one further duplicate script-failure row and two
  cancellation probes that timed fixture-owned session-ID and input-directory
  gates rather than a distinct customer outcome. That also removed the custom
  generator/walker edges from the shared graph. Five normal repeats passed in
  18.852s; three race repeats passed in 39.235s under host variance.

- [x] `transport/http/server` — **9.899s across five current runs (1.980s/run);
  three focused race runs took 12.972s (4.324s/run).** One
  package HTTP fixture covers request/response behavior. The isolated graph
  owns production listener startup, pprof opt-in, bind failure, shutdown, and
  active-stream closure. The OpenAPI operation reachability matrix was removed:
  it enumerated handler registration and route shape rather than a customer
  experience, and its `listModels` inventory row alone consumed about 5s.
  Focused unknown-route, wrong-method, content-negotiation, dashboard asset,
  diagnostics, and listener-lifecycle customer contracts remain parallel. The
  diagnostics witness now samples opt-in heap behavior instead of running
  timed CPU/trace and every-profile inventories. Independent production
  diagnostics modes and shared explicit-session HTTP journeys overlap. The
  first parallel run exposed scenario cleanup that counted every peer's active
  provider route as its own leak; those package-global fixture assertions were
  removed while per-scenario route, stream, session, and filesystem cleanup
  remain.
  The final package-level process/listener/root construction census was also
  removed because it asserted fixture topology after the customer scenarios.
  Direct listener closure, refusal/rebind, response, and stream behavior remain
  at their owning scenarios. A fresh normal run took 2.186s; three race runs
  passed in 20.880s.

- [x] `factory/packaged/fix` — **14.373s re-audit baseline; three optimized
  repeats took 24.550s total (8.18s average); race 18.107s.** One hosted shared
  process covers explicit-session packaged behavior. Provider failure and
  missing/unsafe worktree customer outcomes moved from the serialized local CLI
  fixture into independent explicit sessions; the package's existing focused
  CLI/API parity cell preserves the customer command contract. The synthetic
  resource census, cleanup-path matrix, and test-process forced-unwind report
  were removed because they asserted harness bookkeeping and required every
  scenario to populate internal plan/event ledgers. Direct public session
  deletion, selector release, root removal, listener shutdown, and customer
  response/replay assertions remain. The extra build now serves only the
  focused local CLI parity and real local Git/worktree cases whose command and
  filesystem ownership are customer-visible.

- [x] `transport/cli/output` — **9.503s focused re-audit baseline; five hosted-
  session repeats took 14.046s total (2.809s average); focused race 7.078s.**
  The package fixture covers ordinary JSON/NDJSON
  output. Four isolated graphs inject mutually exclusive stream boundaries: a
  slow writer, a failing writer, an incremental text observer, and
  interruption/startup lifecycle controls. The ordinary tests call
  `t.Parallel`, but the shared fixture holds a mutex across every
  `Process.Execute`; loaded leaf times were therefore queue time, not intrinsic
  customer execution. An unlocked local proof failed with `~default` already-
  bound and missing-session errors, confirming a real default-session authority
  deficiency. The ordinary JSON, NDJSON, quiet-text, and incremental-text
  commands now use one hosted application with a private explicit Factory
  Session per scenario; provider effects route by Factory Session identity and
  no mutex spans `Process.Execute`. Pre-runtime output/argument validation also
  takes no runtime lease: the two customer failures fell from an apparent
  **5.93s** to **0.02–0.04s**. During conversion, a test-only sequential session
  ID failed durable validation; the fixture now uses production identity
  generation rather than weakening the public format. Obsolete mutable effect-
  counter routing, package-global route/active cleanup inventories, and
  duplicate argument validation cells were removed. Writer failure,
  interruption, and production-listener lifecycle contracts retain isolated
  graphs because their customer behavior owns the local command or process
  lifecycle.

- [x] `factory/definitions` — **10.582s across five current runs
  (2.116s/run); three focused race runs took 16.137s (5.379s/run).** An earlier loaded race sample was
  42.411s while sharing the host with four other race packages. A shared
  process covers the ordinary definition/customer CLI cohort. Separate
  service-host and initialization graphs own API hosting, export/import
  persistence, and injected provider-default resolution. The functional suite
  no longer contains the service-root execution-catalog matrix, detached
  snapshot implementation pipeline, service-root flatten/expand duplicate, or
  reference-example inventory. Public CLI validation, flatten/expand,
  import/export, initialization, runnable scaffold, and provider-default
  experiences remain. Static CLI/API validation and preview cells now overlap
  and pass repeated/race execution. A continuation profile separated
  `t.Parallel` pause time from active leaf time and showed no customer leaf over
  one second; ten isolated init, import/export, and provider-default top-level
  tests were simply running serially. They now overlap. The sole process-global
  `t.Setenv` use was replaced with an invocation-owned environment that removes
  ambient provider/model defaults while preserving the test-owned profile.
  The two independent service hosts and init client also close concurrently.
  A later full-lane race run exposed a phase-accounting defect in the defaults
  and preset API scenario: its hosted-readiness clock included an unrelated
  first-run installation of every packaged Factory. Under loaded race execution
  that setup was canceled while owning the `@you/tts` staging lease, producing
  a misleading active-owner diagnostic. The scenario now completes its
  isolated customer-home bootstrap through the same reusable process before
  the hosted invocation starts. Ten normal repetitions pass in **17.185s** and
  ten race repetitions pass in **46.793s**, without padding the readiness
  deadline or serializing the scenario.
  The environment-triggered intentional-
  failure/forced-unwind report was removed because it inventoried fixture
  process, listener, directory, and session cleanup rather than customer
  behavior.

- [x] `factory/packaged/goal` — **5.900s in the final canonical loaded lane;
  five race repeats took 16.145s and the full bounded race lane reported
  14.755s.** The seven shared packaged scenarios own unique explicit sessions
  and thread-safe provider routes and now overlap. The hosted application's
  Current Factory is deliberately idle; each scenario opens its own packaged
  Goal session. This prevents startup Work from consuming the fallback provider
  outcome intended for the pause/cancel/recovery scenario, which was the real
  loaded-lane flake exposed by broad verification. The package-global process,
  selector, Work, root, and session ledger was removed because it asserted
  internal fixture topology rather than customer behavior. Public close,
  absence, root removal, listener shutdown, progress, classification, and
  output guarantees remain. Twenty repeated shared-parent runs and five race
  repeats pass. The focused quiet CLI cell remains `~default`-owned until local
  explicit-session invocation exists.

- [x] `factory/packaged/factory_builder` — **8.42s focused re-audit baseline;
  five optimized runs took 33.474s total (6.695s average); focused race
  12.611s.** Five independent customer journeys were nested under a joining
  parent and made parallel. Each owns a private home, Factory definition,
  explicit Factory Session, provider route, and generated destination, so the
  shared application does not retain an invocation-wide lock while a journey
  waits. The package's process/session/root census and persisted-row/artifact
  census were removed because they measured fixture topology and cleanup
  bookkeeping rather than customer behavior. Public session deletion remains
  observable by proving the closed session is unreadable; generated graph and
  JavaScript Factories are still installed and invoked through their public
  commands. The remaining roughly seven-second package time is dominated by
  constructing the reusable application, hosting it, and the final installed-
  artifact command checks; the parallel customer leaves themselves complete in
  0.29–0.57s. Further improvement requires profiling that setup/teardown path,
  not weakening the retained generation-and-use journey.
  A follow-up trace separated those phases: the five hosted-session journeys
  still finish in under one second, while three post-host commands prove that
  the generated or preserved Factories actually run. Attempting those local
  commands concurrently on the reusable process blocked instead of completing,
  despite distinct homes, demonstrating a process-wide local invocation
  boundary. That attempted optimization was reverted. The restored focused
  package passed in 5.569s; changing this phase requires explicit local session
  authority, not more `t.Parallel` declarations.

- [x] `factory/replay_contracts` — **10.208s re-audit baseline; three optimized
  repeats took 19.569s total (6.52s average); race 14.608s.** The re-audit
  rejected the earlier claim that every graph represented customer-visible
  replay behavior. Direct Work-snapshot reader capture, Visualization-root
  activation, selected-tick reconstruction through a Recordings observer, and
  canonical Append/Subscribe calls through an injected service observer were
  service/wire tests in the functional directory. Equivalent owner-level tests
  already cover those contracts, so the functional copies were removed.
  Public record/replay through `Process.Execute`, malformed replay rejection,
  durable historical result/dispatch reads, and customer projection behavior
  remain. Their distinct recording effects and reconstruction boundaries still
  justify separate graphs.

- [x] `bootstrap_portability` — **10.422s re-audit baseline; three optimized
  repeats took 11.528s total (3.84s average); race 12.111s.** Pure authored
  fixture interpolation, canonical route-array shape, flattened bundle shape,
  expanded filesystem layout, and fixture-generated contract comparisons were
  removed from the functional lane. A second “integration smoke” inside the
  functional package duplicated the retained expanded-layout dispatch-ready
  customer journey and was removed. Independent public export/import,
  activation, starter-Work, portable-layout, thin-runtime, nested-doc, and
  Current Factory scenarios now run in parallel. The remaining Automat
  dispatch-ready and invalid-definition cells stay serial because they
  temporarily control process-wide PATH/current-directory state; their public
  execution outcomes remain customer-visible.

- [x] `orchestration/javascript/loading` — **12.121s re-audit baseline;
  10.606s in the newest loaded lane; three optimized repeats took 15.963s
  total (5.32s average); focused race 17.279s.** The shared fixture covers
  customer CLI/API source loading. The isolated partial-start graph and its
  process/listener/stream/route/root/worktree census were removed because they
  tested injected lifecycle-role bookkeeping rather than source-loading CX.
  Every
  completed local one-shot scenario previously issued a second
  `Process.Execute(session terminate …)` during cleanup even though the
  invocation-local session had already been released; that redundant harness
  command was removed. API-owned sessions still use the public lifecycle
  endpoint. Missing-import, TypeScript syntax, and TypeScript source-map tests
  no longer repeat the same later-success recovery invocation; the inline
  syntax case retains that customer recovery guarantee once. Those three now
  purely pre-runtime diagnostics overlap safely. Package-global build/start/
  stop/session/stream inventories were also removed from cleanup. Retained
  successful CLI loading cells bind `~default` and remain serialized until the
  local command accepts explicit session authority.
  Reusing the serialized fixture home reduces repeated successful-source setup,
  but a follow-up showed that sharing it across the three parallel diagnostics
  creates packaged-installation lock contention and inflates each leaf. Those
  parallel diagnostics therefore retain scenario-owned profiles. This is the
  concrete reason the testing standard now distinguishes reusable process
  wiring from reusable mutable invocation state. The plain TypeScript syntax
  row was then removed because the retained source-map row proves the same
  customer syntax code and authored line while additionally proving remapping;
  it was not a distinct customer journey. Three post-change normal runs took
  21.106s (7.035s/run), down from the 9.717s loaded cohort sample, and race
  passed in 22.638s.

- [x] `models/root_composition` — **7.635s re-audit baseline; three current
  full repeats averaged 8.91s under host variance; race 19.385s.** The slowest
  leaf was not the shared-session overlap ledger: it sequentially requested
  unavailable TTS, unavailable ASR, and an unknown model through the same
  managed-readiness boundary, costing about two seconds per request. Parallel
  scheduling showed that this boundary still queues the requests internally.
  TTS and ASR were the same known-built-in-unavailable customer class, so the
  ASR inventory permutation was removed; one known built-in and one unknown
  reference remain. The focused leaf fell from 6.06–6.90s to 5.30s. The
  package otherwise already parallelizes customer catalog, diagnostic, and
  LocalAI cells. The second construction remains the asserted pull-to-ready
  process-reconstruction behavior.
  The follow-up customer-boundary pass removed the HTTP and combined HTTP/CLI
  every-operation conformance matrices. Focused ASR, TTS, embedding, multimodal,
  failure, and parity stories already protect the distinct customer journeys;
  enumerating the catalog twice was inventory coverage. Two further remote
  missing-cache stories duplicated the retained coded local/remote diagnostics
  and were removed. The remaining empty-edge diagnostic commands now share one
  application graph while retaining distinct profiles and endpoints. Three
  normal repeats passed in 22.479s (7.493s/run under host variance), and the
  race run passed in 15.843s.

- [x] `orchestration/petri/dispatch` — **7.757s focused before the follow-up;
  three post-change repeats took 22.882s (7.627s/run).** Customer dispatch,
  concurrency, correlation, failure, lineage, and panic journeys already use
  one hosted graph plus the incompatible panic edge. The package nevertheless
  maintained and printed a 40-plus-row process/session ledger after the tests.
  That ledger asserted construction counts, unique fixture paths, and cleanup
  bookkeeping rather than customer behavior, so it was removed. The nearly
  unchanged time proves the ledger was output noise rather than the critical
  path; public session closure and terminal Work assertions remain.

- [x] `factory/packaged/subagent` — **6.849s focused before the follow-up;
  three optimized repeats took 19.910s (6.637s/run), a fresh verbose sample
  took 5.833s, and race passed in 11.969s.** The package already owned one
  reusable graph, but all seven customer scenarios ran sequentially. Three
  local CLI journeys still require `~default` ownership and remain serial.
  After the host starts, failure, response-stream, explicit-effort, and
  provider-default journeys now use independent explicit sessions and run in
  parallel. The package lifecycle census and the extra sessions opened solely
  to satisfy its expected-count ledger were removed; each real hosted scenario
  still closes its own public session and proves it is no longer readable.

- [x] `factory_definitions/transports/cli/yaml_parity` — **8.460s re-audit
  baseline; three optimized repeats took 15.576s total (5.19s average); race
  14.131s.** The success matrix previously ran seven equivalent JSON, YAML,
  YML, file, directory, and explicitly selected path permutations. It now
  retains representative JSON-versus-YAML validate, flatten, and run parity;
  supported aliases and root-discovery shapes are parser/contract concerns,
  while ambiguous-directory rejection remains a distinct customer failure.
  Six rejected-source rows also built six fresh root processes solely to count
  zero provider calls. They now use the package-owned shared process and assert
  the same actionable pre-runtime diagnostics, eliminating those repeated
  builds without introducing a mutable provider selector. YAML create/update
  canonical persistence remains a separate customer journey.

- [x] `sessions/root_composition` — **24.312s across three runs (8.104s/run),
  race 18.839s.** The package-shape inventory, fake-owner detached-service
  matrix, constructor-inertness probe, and fixture-cleanup self-test were not
  customer behavior and were removed. The reusable-process customer scenario
  no longer asserts constructor counters. The three P3/P7 customer journeys
  own independent processes and now run as parallel leaves; retained recording,
  redaction, runtime-opening, replay, cancellation, and recovery journeys keep
  their distinct immutable edge graphs.

- [x] `workers/concurrency` — **1.571s clean; 8.905s across five runs
  (1.781s/run); race 4.647s.** The malformed-definition validation probe and
  constructor/provider-registry checks were duplicate internal coverage and
  were removed, leaving one shared process for the customer concurrency
  matrix. Capacity, cross-session concurrency, cancellation, timeout,
  recovery, and forced cleanup now overlap. Package-global active-call
  assertions initially made this unsafe; replacing them with each scenario's
  route-owned runner observations removed that accidental coupling while
  preserving the concurrency proof.

- [x] `sessions/execution` — **11.177s across three current runs (3.726s/run),
  focused race 11.046s; 3.896s in the loaded three-package slice.** The
  continuous hosted-idle customer proof now
  synchronizes on listener binding and immediately verifies that the invocation
  remains live before customer cancellation. The former server/site rows each
  held an otherwise independent fixture for an extra fixed 500ms negative-
  liveness observation. That delay supplied no stronger completion signal and
  was removed. Timeout timers remain only as failure ceilings around actual
  readiness and completion signals.

- [x] `factory/visualization/runtime_metrics` — **7.196s across three runs
  (2.399s/run); race 6.523s.** The committed-fixture shape inventory and direct
  platform reader/opener/coordination/reservation matrices were unit and lint
  concerns disguised as functional coverage and were removed. The artifact
  journey now observes the configured customer lifecycle and its test-owned
  filesystem output only. Priced and unpriced replay journeys remain isolated
  servers, but their short CLI reads reuse the package CLI process instead of
  building two additional graphs, and the independent customer journeys run
  in parallel.

- [x] `transport/mcp/protocol` — **5.178s across five runs (1.036s/run),
  race 4.646s.** A full-lane run exposed a five-minute initialization hang,
  not a shutdown delay: the isolated server inherited the developer home and
  waited on another package's packaged-installation ownership. MCP invocations
  now own an isolated home/profile, and protocol response reads have a bounded
  failure guard that closes the pipe instead of leaking a blocked reader. The
  harness topology census was removed; malformed parameters, canonical
  missing-session errors, initialization, and clean stdio shutdown remain.

- [x] `transport/mcp/stdio` — **12.065s across five runs (2.413s/run), race
  9.851s.** Fixture-backed initialization now owns an isolated home rather
  than contending through ambient operator state. The `TestMain` inventory of
  roots, invocations, contexts, streams, and temporary directories was removed
  because it tested the harness. Customer initialization, tool discovery,
  canonical errors, runtime initialization, incomplete-frame termination, and
  stdio shutdown remain.

- [x] `transport/cli/commands` — **23.459s across five current runs
  (4.692s/run); three focused race runs took 35.837s (11.946s/run).**
  The full-lane stdin failure was packaged-installation contention in the real
  developer home, not a primary-result mismatch. The reusable in-process CLI
  harness now supports a copied default invocation environment, and this
  package assigns a test-owned profile without mutating process-global
  environment state. The failing stdin journey passed ten focused repetitions
  before the full package passed three repetitions. The packaged-docs functional
  coverage now uses one shared graph for topic discovery and actionable unknown
  topic failure. Its former every-topic runtime inventory belongs to the docs
  smoke/contract lane and was removed; the docs leaf fell from about 1.08s to
  0.28s. The shared remote group now uses one concurrent reusable root and one
  service host. Its invocation-owned and explicit-session customer scenarios
  overlap; only default-session mutation and process-environment
  characterization remain serialized. Five focused shared-remote runs took
  9.133s (1.827s/run), down from roughly 3.5s/run. The first overlap probe also
  found that a deferred parent server cleanup ran before paused parallel
  subtests; test-owned cleanup now keeps the host alive through all children.
  The `run` leaves retain separate graphs because they execute concurrently
  with invocation-owned local runtimes.

  The clean post-audit lane later measured 8.247s under two-package load. Its
  named-Factory parent still hid two independent process-owned customer
  journeys behind sequential subtests; those leaves now overlap. The clean-
  stdout journey retained two successive invocations, which prove reuse and
  pipeability, and removed a third stdin permutation already covered by the
  dedicated stdin customer test. Five post-change runs took 29.758s total
  (5.952s/run) under host variance, and three race runs took 30.103s
  (10.034s/run). The successful local runtime leaves still own separate graphs
  because the reusable application lacks explicit local session authority;
  pre-runtime validation graph convergence remains a smaller follow-up if this
  package re-enters the loaded critical path.

- [x] `sessions/chat_sessions/root_composition` — **16.641s in the latest loaded
  lane; 15.319s focused.** Three consecutive focused runs completed in
  **40.663s** (**13.554s/run**) and the focused race run passed in **37.571s**.
  The slowest four customer leaves report roughly
  10 seconds each because their `t.Parallel` declarations queue behind a
  process-global ACP home/settings lease; their useful work overlaps only for
  scenarios sharing one active home. This is a production testability and
  customer-concurrency deficiency, not a customer invariant. The package no
  longer contains its roughly 800-line root/process/connection/pipe/session/
  turn/call/peer/path resource census or the no-op hooks that fed it. Cleanup
  retains only owned process, public Chat Session, stream, and temporary-file
  boundaries. The remaining home/session-authority repair is explicitly planned
  in `docs/internal/plans/default-session-authority-for-concurrent-invocations.md`;
  changing that production contract is intentionally outside this mechanical
  functional-test optimization pass.

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

- [x] `recordings/process` — **7.837s in the refreshed loaded lane; the
  internal steady-state byte-volume cell took 2.36s focused. Five optimized
  runs took 21.954s total (4.391s/run); focused race 11.674s.** The removed
  cell submitted two identical events, counted internal recording writes and
  bytes, then slept 500ms to prove the artifact did not change. It had no
  customer observer. The in-process orderly-stop journey retains the customer
  guarantee that the public recording is readable and final before
  `Process.Execute` returns. Windows graceful-stop and taskkill durability are
  real executable/signal boundaries and remain identified for integration
  reclassification rather than being treated as functional parallelism
  candidates.

- [x] `acceptance` — **8.169s in the refreshed loaded lane and 6.03s focused;
  five optimized runs took 13.095s total (2.619s/run); focused race 6.602s.**
  Multi-command config/provider/model/named-Factory journeys used a one-shot
  harness that rebuilt the complete application for initialization and every
  subsequent command. Each journey now owns one reusable process, and the
  independent homes, ports, files, and processes run as parallel top-level
  tests. Opt-in intentional-failure cleanup reporting, harness-specific error
  wrapper assertions, and unused-port reservation were removed because they
  tested the acceptance support itself. The public two-home isolation journey
  remains because cross-home leakage is customer-visible.

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

AGY fell from **23.498s** initially and **12.704s** in the prior loaded lane to
**2.341s** focused after the deeper blocking repair. Five consecutive runs now
pass in **7.004s** (**1.401s/run**) and the focused race run passes in
**5.666s**. Direct, golden, lifecycle, recovery, and packaged-review customer
scenarios share one hosted application, open unique explicit Factory Sessions,
route by immutable execution scope, and run in parallel. An idle host Factory
prevents application startup from consuming any customer scenario's seeded
Work or fake-provider outcome.

The earlier audit stopped after seeing that direct and golden commands bound
the process-local `~default` Factory Session. That correctly identified the
conflict but incorrectly treated explicit sessions as dependent on a future
production change. Opening the supported public session directly on the
already-hosted application removed both the default binding and the fixture
mutex without weakening the customer proof. The probe also exposed two
repeat-only fixture defects: the
failure-then-recovery route exhausted its outcome sequence, and lifecycle gates
were package-lifetime one-shot channels. Recovery now resets per scenario, the
two-route overlap proof uses scenario-owned gates and public Work, Factory
Event, and response-event observations. The internal early-host-exit census and
its obsolete per-invocation hosted-command scaffolding were removed.
The newest loaded lane measured **4.411s** while running alongside another
functional package.

The first full race lane after this migration exposed one remaining false wait:
the direct cancellation cell opened a response stream it never asserted and
then treated a response frame as mandatory, although cancellation guarantees
terminal session, Work, and Factory Event state rather than a response frame.
The resulting 30-second wait was removed by subscribing only in the successful
response-stream scenario. Ten cancellation race repetitions passed in
**6.972s**, three complete AGY race repetitions passed in **8.304s**, and the
loaded provider/provider-session race cohort passed with AGY at **6.040s**.

Named invocation measured **8.20s** before its standards cleanup and **8.469s**
in the preceding loaded lane. After hosted-session conversion, five current
runs pass in **16.374s** (**3.275s/run**) and three focused race runs pass in
**18.419s** (**6.140s/run**). Preparation failures run in parallel and retain the stable
customer diagnostic, sensitive-input redaction, and proof that execution never
reaches the provider. Package-wide inventories of session IDs, runtime hosts,
submission/dispatch counts, recording filesystem operations, listener starts,
process constructions, and router cleanup were removed because they described
fixture internals rather than customer behavior. Successful named and explicit-
file CLI parity cases now share one hosted application, open unique explicit
Factory Sessions, route provider effects by immutable execution scope, and
overlap. The raw recording-JSON decoder and canonical submission-record shape
assertions were also removed: recording/replay customer behavior belongs to its
public recording suites, while argument normalization shape belongs to contract
coverage. Provider prompts still prove resolved positional, repeated, default,
file-content, and stdin customer inputs with no unresolved interpolation.

Packaged full flow previously measured **7.3–8.0s/run**. Four independent
customer journeys now overlap on the package's shared process, provider routes
are scoped only to each scenario's Factory directory, and an artificial 75ms
implementation delay was removed. The internal process/session/listener and
recording-operation inventories were removed because they tested fixture
topology rather than customer behavior. Five current runs pass in **14.793s**
(**2.959s/run**) and three focused race runs pass in **16.129s**
(**5.376s/run**).

`providers/acp` fell from **10.403s** in the loaded lane to **4.542s** in the
new focused run. Four independent crash/disconnect/retry/unknown-provider
customer journeys had run serially before the already-parallel OS-peer cohort;
they now overlap with that cohort. The packaged conformance test no longer
counts and decodes every generated profile fixture before running one shared
implementation—it retains one representative packaged customer execution,
while generated profile shape belongs to provider-catalog contract/lint
coverage. One initial five-repeat stress command failed intermittently; the
immediate five-repeat rerun passed in **29.786s**, ten further repeats passed in
**67.626s**, and the focused race run passed in **17.545s**. The controlled
full-lane rerun remains the loaded flake gate.

The package still recursively launches the compiled Go test executable at 13
sites to prove ACP stdio, process crash, executable lookup, and pipe behavior.
Those are valid integration guarantees but violate the current functional
standard's no-executable rule. Removing their remaining roughly four-second
parallel plateau requires moving the OS-peer cohort to an integration package
with an allowed shared support boundary; it is not a reason to serialize the
customer scenarios again.

The root `providers` package measured **10.746s** in the newest loaded lane.
Two serial tests named `IntegrationSmoke` recursively launched the compiled Go
test executable, created sleeping descendants, and asserted process-tree
termination before the actual parallel functional cohort could run. They were
removed from the functional lane: controlled-boundary timeout requeue/retry and
canonical command cancellation remain here, while real descendant cleanup is a
platform/integration property. Five focused package repeats now pass in
**15.712s** total (**3.14s average**) and the focused race run passes in
**7.540s**.

`sessions/restart` was re-profiled at **8.384s**. Its three in-process customer
restart/resume scenarios previously ran serially even though they own separate
Factory directories, profiles, servers, and provider fakes. They now use
test-owned profile environments and run in parallel; three package repetitions
pass in **22.958s** and the race run passes in **39.226s**.

The remaining package critical path is not functional execution: three board
persistence cases recursively launch the compiled Go test executable to prove
daemon restart, hard-kill, signal, and process-exit behavior. Together those
leaves account for roughly **6.2s** of the measured package. The testing
standard classifies that proof as integration coverage. A direct relocation
probe showed that the suite currently imports the functional-only support
package, so moving it requires a shared integration-support boundary (or a
compiled-CLI integration fixture); it must not be made “parallel” inside the
functional lane or copied around the Go `internal` boundary.

`work/submission` measured **11.011s** in the loaded lane because ten
independent customer journeys each opened an isolated server serially. Every
journey owns its Factory directory, home/profile, provider fake, request IDs,
and server lifecycle, so the top-level tests now run in parallel. Five repeats
pass in **20.620s** total (**4.12s average**) and the focused race run passes in
**10.858s**. Ordered child steps that intentionally share one customer Factory
Session remain serialized inside their owning journey.

`workers/transports/http` measured **11.037s** in the loaded lane with eleven
independent server journeys serialized. Ten retained customer scenarios now
run in parallel and five repeats pass in **11.570s** total (**2.31s average**);
the focused race run passes in **6.925s**. The removed eleventh test submitted
exactly 32 sequential Works and checked first/middle/last associations plus
aggregate event counts. That was a characterization inventory/load loop, not a
new Worker Session customer contract; focused route association, repeated
read stability, concurrent reads, pause/resume, lifecycle control, shutdown,
and remote invocation coverage remains.

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
- [x] The complete functional lane passes after the newest home-isolation and
  MCP changes with package concurrency capped at two. MCP protocol completed
  in **1.346s**, MCP stdio in **3.574s**, CLI commands in **7.308s**, Workers
  concurrency in **1.496s**, and Sessions root composition in **5.239s**.
- [x] The race detector passes across `./tests/functional/...` with `-p 2`,
  `-count=1`, and a five-minute per-package timeout. Under full-lane race
  contention, CLI output completed in **8.230s**, Factory Builder in
  **13.698s**, Workers inference in **11.592s**, and Workers CLI lifecycle in
  **15.164s**; no detector finding or package timeout occurred.
- [x] The post-standards bounded race rerun emitted a passing result for every
  functional package. The corrected `factory/definitions` package completed in
  **9.294s**, Provider Sessions CLI in **8.120s**, ACP stdio in **4.384s**,
  Workers inference in **14.357s**, and Workers CLI lifecycle in **16.349s**.
  After the final package result, the Windows Go driver remained resident with
  no test executable or child process; the inert driver was stopped separately
  and is not reported as a package cleanup or race failure.
- [x] A post-change timing report is captured: approximately **296 seconds**,
  approximately **48 seconds faster** than the 344.310-second audit sample and
  approximately **31 seconds faster** than the immediately preceding 327.303-
  second run. The hosted explicit-session CLI output conversion passed under
  full-lane contention in **3.471s**.
