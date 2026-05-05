# Backend

## Core Loop

The backend centers on a deterministic tick loop that updates a shared world state from submitted events. Each tick reads pending inputs, applies subsystem logic, and emits outputs that are handed off to queues and workers.

```mermaid
flowchart LR
    cfg[Config Load]
    qin[Inbound Event Queue]
    tick[Core Loop Tick]
    world[World State]
    subs[Subsystems]
    out[Output Planning]
    qout[Dispatch Queues]
    agents[Agents and Workers]
    feedback[Generated Events]

    cfg --> world
    qin --> tick
    world --> tick
    tick --> subs
    subs --> out
    out --> qout
    qout --> agents
    agents --> feedback
    feedback --> qin
    tick --> world
```

The loop is intentionally closed: workers and agents do not mutate the world directly. They produce outputs that re-enter the system as events, which keeps the state transition history explicit and replayable.

## Event Stream

The world is derived from an ordered event stream rather than a collection of opaque mutable objects. This stream is the durable source of truth for replay, synchronization, and historical inspection.

```mermaid
flowchart TB
    subgraph Sources[Event Sources]
        e1[Config Initialization]
        e2[Work Submissions]
        e3[Work Transitions and Dispatches]
        e4[World Configuration Changes]
        e5[Loop State Changes]
    end

    stream[Ordered Event Stream]
    replay[Replay Engine]
    snap[World State at Tick N]
    hist[Historical World State at Tick X]
    customer[Customer or Consumer View]

    e1 --> stream
    e2 --> stream
    e3 --> stream
    e4 --> stream
    e5 --> stream
    stream --> replay
    replay --> snap
    stream --> hist
    snap --> customer
    hist --> customer
```

At any tick, the current world is the composition of all prior events. Because the stream is deterministic, customers can receive the same event history and reconstruct a consistent view at any chosen timestamp or tick.

# Front End

The frontend is an embedded React application that consumes the backend event stream and derives a customer-facing world view from it. The UI emphasizes composable dashboards and visualizations rather than owning the authoritative system state.

## Frontend Composition

```mermaid
flowchart TB
    react[React App]
    state[Client Event Store and View State]
    world[Derived World View]
    grid[React Flow Grid and Card Layout]
    charts[Charts and Summaries]
    flows[Flow Visualizations]
    detail[Work and Factory Detail Panels]

    react --> state
    state --> world
    world --> grid
    grid --> charts
    grid --> flows
    grid --> detail
```

The React layer receives events, derives projections for the current world, and renders that state through cards, charts, and flow-oriented views.

## Frontend and Backend Integration

The frontend and backend are connected by an event-oriented contract. The backend owns execution, scheduling, and replayable history; the frontend subscribes to that history, derives projections, and sends user actions back as submissions.

```mermaid
sequenceDiagram
    participant User
    participant UI as React Frontend
    participant API as Backend API
    participant Engine as Core Loop Engine
    participant Stream as Event Stream

    User->>UI: Submit work or inspect state
    UI->>API: Send command or request
    API->>Engine: Normalize into domain event
    Engine->>Stream: Append emitted events
    Stream-->>Engine: Replay into latest world state
    Stream-->>UI: Stream events or snapshots
    UI->>UI: Rebuild derived world view
    UI-->>User: Render updated dashboard
```

This split keeps the frontend lightweight and keeps the backend authoritative. The same event stream that powers execution can also power dashboards, audit history, and deterministic replay.
