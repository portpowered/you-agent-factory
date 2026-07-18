# `@you-agent-factory/factory-replay`

Framework-independent, deterministic Factory-event replay primitives.

The package accepts public `FactoryEvent` aliases from
`@you-agent-factory/client`, canonicalizes event history by logical tick,
sequence, event time, and id, accepts each event id once, and reconstructs a
caller-defined Factory world at either the latest or an explicit historical
logical tick. The caller supplies a pure reducer and projection adapter, so the
package has no React, Zustand, browser, network, session-routing, persistence,
or diagnostics dependencies.

`createFactoryReplayCheckpoint` and `advanceFactoryReplay` support live replay
from an accepted tail. Callers provide state-clone and selected-tick adapters;
the kernel applies only unseen events after the checkpoint tick in canonical
order and never mutates the supplied checkpoint.
