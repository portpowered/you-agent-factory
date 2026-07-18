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

`projectFactoryTopologyAtTick` reconstructs the latest public Factory topology
at or before an explicit logical tick. `projectFactoryTopology` projects an
already selected `FactoryDefinition`. Both operations return deterministically
ordered resource, worker, workstation, Work Type, and Work State nodes plus
canonical connections. Node and connection IDs follow the public graph ID
contract, and connection endpoints include renderer-compatible handle IDs.
References that cannot be resolved are omitted from the connection collection
and returned as structured projection issues, so consumers never receive a
dangling edge.

Canonical initial and replacement topology events retain durable IDs for
resources, workers, workstations, Work Types, and Work States, plus authored
worker and workstation resource requirements. This lets recordings preserve
stable graph identity and both resource connection kinds even when public
entity names change. Internal resource availability arcs present in canonical
workstation IO are excluded from public Work-State routes and do not create
false unresolved-connection issues.

`projectFactoryActivityAtTick` reconstructs active customer Dispatches,
affected workstations, consumed Work IDs, workers, and resource occupancy from
canonical event history. Consumers that already hold selected replay evidence
can call `projectFactoryActivity` directly. Occupancy reports known occupied
and available quantities, or explicitly reports unavailable evidence when an
active Dispatch lacks resource facts; it never turns missing evidence into a
fabricated zero occupancy.

`projectFactoryWorkProgressAtTick` reconstructs known customer Work and active
Dispatch consumption at an explicit logical tick, then assigns every Work ID
to exactly one of failed, completed, active, queued, or unclassified. The
classification applies that precedence once, excludes internal time Work, and
uses `unclassified` when partial event or topology evidence cannot safely
determine a lifecycle category. `projectFactoryWorkProgress` exposes the same
pure partition for consumers that already hold selected replay evidence.

The hosted dashboard projects these three public read models from its selected
checkpoint-safe replay state and exposes them together on the timeline world as
`factoryReplay`. The production timeline projector also derives the legacy
dashboard topology/runtime compatibility fields from the shared topology,
activity, occupancy, and Work-progress decisions. Petri-shaped structures
remain reducer evidence and supplemental dashboard diagnostics, not a second
public topology or progress classifier.
