# Factory emulator event sink

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
