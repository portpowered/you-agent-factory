# `@you-agent-factory/factory-emulator`

This package publishes the stable v1 JSON Schema and generated TypeScript
contract for deterministic Factory-emulator scenarios. The schema is available
as raw JSON from `@you-agent-factory/factory-emulator/schema`; the root module
exports `scenarioSchema` and `SUPPORTED_SCENARIO_VERSION`.

Every scenario declares a version, id, deterministic seed, UTC `startAt`,
ordered rules, and explicit unmatched behavior. Rules use finite scripted
outcomes with explicit exhaustion behavior. Initial submissions and lineage
cursors are structurally represented here; the parser validates the scenario
shape and supported Factory execution subset before emulation begins.

`activityLabel` is optional, limited to 120 characters, and only represents
transient emulator activity. It is never canonical Factory event content.

The schema under `contracts/factory-emulator/` is authored source. Run
`npm run generate` from this package to regenerate its committed schema and
TypeScript artifacts. The TypeScript declarations are derived from the schema's
properties, required fields, references, variants, and documented runtime
constraints rather than a separately authored type template. Run
`make generate-emulator` to verify they are current; when it reports drift,
regenerate them and commit the resulting artifacts.

## Parsing a scenario for browser emulation

`parseEmulatorScenario(scenario, factory)` is a pure preflight boundary. It
returns either the typed authored scenario and supported Factory definition, or
structured diagnostics with a stable JSON Pointer `path`, `code`, `message`,
and `expectation`. It never starts emulator activity or creates Factory events.

The v1 emulator accepts static Petri Factory topology only. It rejects
JavaScript orchestration, configured Factory resources or guards, and cron,
repeater, or poller workstation scheduling before emulation starts. This keeps
scripted browser behavior deterministic while the runtime support subset grows.

## Rule semantics

Rules are evaluated in authored order; `selectEmulatorRule` returns the first
matching rule and no later rule can change that result. Parsing rejects a later
rule only when an earlier rule provably covers its supported domain, including a
known initial-submission id covered by an earlier work-type rule. It deliberately
does not report uncertain overlap as shadowing.

`resolveEmulatorScenarioResult(scenario, submission, invocationIndex)` is a
pure helper for a zero-based invocation count of the selected rule. It returns a
scripted outcome while one is available, repeats the final outcome only for
`repeatLast`, delegates only `useUnmatchedBehavior` exhaustion to the explicit
unmatched behavior, and otherwise returns the explicit exhaustion rejection.

Initial submission ids and rule ids must be unique. Initial submissions and
work-type matchers must name Factory work types. Lineage cursors may target one
known initial submission or one earlier `complete` scripted outcome; missing,
forward, cyclic, and incompatible targets are rejected with diagnostics before
emulation begins.

## Inspecting support and using examples

`inspectEmulatorSupport()` returns the stable machine-readable v1 capability
report. It covers the accepted static Factory subset, explicitly unsupported
Factory behavior, matchers, outcomes, lineage cursors, exhaustion and unmatched
options, initial-submission constraints, and `activityLabel` limits. The parser
uses that same Factory support policy, so the report cannot advertise execution
capabilities that parsing rejects.

`emulatorScenarioExamples` exports a minimal scenario and a multi-rule scenario.
The latter shows ordered priority matching, initial submissions, a scripted
lineage cursor, finite exhaustion, explicit unmatched rejection, and transient
`activityLabel` metadata. Both examples are validated through the public parser.

## Event sink and logical tick runtime

`@you-agent-factory/factory-emulator` provides the transport-neutral,
caller-owned `FactoryEventSink` contract used by Factory emulator hosts.

`write` accepts one ordered canonical `FactoryEventBatch` asynchronously. Its
only successful receipt means every event in the supplied order was accepted;
partial success must reject. `close` is a separate asynchronous operation and
does not implicitly accept a pending write. The values crossing the contract
are data-only, structured-cloneable Factory event and receipt shapes, making
the contract suitable for a future worker or process boundary.

`createMemoryFactoryEventSink({ maxEvents })` retains only the newest complete
batches that fit its event limit. `createFactoryRecordingSink({ sessionId,
maxEvents })` produces one finite recording for that session. Both helpers
structured-clone accepted batches and returned history, reject writes after
`close`, and preserve each batch atomically when a recording would exceed its
event bound.

This package intentionally does not own replay history or depend on browser
timers, React, Zustand, dashboard state, or transport code.

`@you-agent-factory/client` is a peer dependency because it supplies the
canonical Factory event and recording types. Consumers install both published
packages; the emulator package does not embed a checkout-relative dependency.
The peer range remains unconstrained while this dependency is type-only and no
minimum compatible client release has been established.

`createFactoryEmulator({ initialState, calculateTick, sink })` provides the
small logical-tick boundary used by an emulator host. It calculates a complete
batch from a detached committed-state snapshot, waits for `sink.write`, and
only then commits the calculated next state. A concurrent `advance` rejects
while the write is unresolved, ensuring no later tick is calculated or
committed ahead of an unaccepted batch.

If a sink rejects a tick, the emulator retains that detached batch and its
calculated state as the only pending transaction. The next `advance` retries it
with the same IDs, timestamps, contents, and order; `reset` explicitly
discards it. `pending` and `status` expose detached recovery state and the last
write or close error. An idle emulator remains open. When configured with
`calculateClose`, `close` writes that caller-supplied terminal lifecycle batch,
waits for it to be accepted, and then closes the sink. A pending rejected tick
blocks close so canonical history cannot be skipped. A rejected terminal batch
is likewise retained unchanged for the next close attempt; after its acceptance
a failed sink close retries only `sink.close`, never the terminal write.

## Deterministic session start and reset

`createFactoryEmulatorSession({ factory, scenario, sink })` owns one long-lived
emulator lifecycle. `start()` revalidates the supported Factory and scenario
before activity, normalizes `startAt` to a UTC instant, then writes an ordered
topology/run bootstrap batch followed by one normalized initial-submission
batch. The returned state becomes visible only after those writes are accepted.

Session, request, trace, Work, and event identities are derived from canonical
Factory/scenario inputs, the scenario seed, authored submission coordinates,
and logical sequence. The same domain-separated derivation is reserved for
internal token, dispatch, and completion identities. It reads no ambient time,
randomness, locale, or process-global counter. Every startup event is stamped at
virtual elapsed time zero (`startAt` after UTC normalization).

`reset()` is available after a successful start. It clears runtime Work,
virtual elapsed time, counters, and rule cursors and returns the session to
`pre-start` without writing an event. Starting again with the same immutable
configuration reproduces the original event bytes and committed snapshot;
changing the seed creates a different deterministic identity stream.

While started, `submit(work)` and `submit([work, ...])` accept the same
`id`, `workType`, and optional data-only `input` shape as scenario initial
submissions. The complete submission is detached and validated before the sink
is called. One accepted batch adds all of its Work in caller order; any invalid
member rejects the command without events, counter changes, or committed Work.
Request, trace, Work, and event identities include stable command coordinates,
so repeated sessions reproduce the same submission stream without relying on
ambient counters.

Scripted `complete` and `reject` outcomes may set a non-negative integer
`durationMs`; omission means zero virtual milliseconds. `advanceToNext()`
commits exactly one scheduler batch: ready Work starts at the current virtual
instant, while the next step jumps to the earliest deadline and completes all
dispatches due there in stable Work order. `advanceBy(durationMs)` processes
each eligible batch through its inclusive target and, when Work remains open,
ends exactly at that virtual instant. Waiting intervals move the virtual clock
without synthetic events, while a fully idle session returns an event-free
no-op receipt and stays open.

All advancement timestamps are derived from normalized scenario `startAt` plus
integer virtual elapsed milliseconds. The session creates no timers and owns no
playback speed, pause, visibility, or replay-selection state.

`status()` derives execution state from session facts. A started session with
ready Work reports `ready`; one with no unfinished Work remains open and reports
`idle`; unmatched unfinished Work configured to be ignored reports `waiting`.
During an accepted submission write it reports `active`, while `state()`
continues to expose only the prior committed snapshot. Once the sink accepts
the batch, the new Work becomes visible and the session reports `ready` or
`waiting` from its executable scenario facts.
