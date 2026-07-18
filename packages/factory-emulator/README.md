# Factory emulator event sink

`@you-agent-factory/factory-emulator` provides the transport-neutral,
caller-owned `FactoryEventSink` contract used by Factory emulator hosts.

`write` accepts one ordered canonical `FactoryEventBatch` asynchronously. Its
only successful receipt means every event in the supplied order was accepted;
partial success must reject. `close` is a separate asynchronous operation and
does not implicitly accept a pending write. The values crossing the contract
are data-only, structured-cloneable Factory event and receipt shapes, making
the contract suitable for a future worker or process boundary.

This package intentionally does not own replay history or depend on browser
timers, React, Zustand, dashboard state, or transport code.
