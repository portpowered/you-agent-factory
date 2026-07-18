# FactoryTopologyReplay

`FactoryTopologyReplay` renders one host-prepared Factory topology and activity
projection. Import it from `@you-agent-factory/components/visualizers` and load
the package `styles.css` entrypoint once in the host.

The required `status` prop is a controlled discriminant. `ready` requires a
prepared `projection`; `loading`, `empty`, and `failed` do not accept one, so
stale graph content cannot remain presented as current. Loading exposes a busy
status, empty explains the absence of prepared topology, and failed preserves
the labeled visualizer region with an alert. A supplied `onRetry` callback adds
a retry action that emits intent only; the host still decides what recovery
means. A closed, successfully loaded static recording is `ready`.

The host owns transport, raw events, replay, selected ticks, checkpoints,
playback, storage, selection, and Factory mutation. The visualizer owns only a
disposable React Flow projection. Pass a new `projection` to replace the visible
tick and pass `selectedNodeId` back after `onSelectNode` to control selection.

The projection contains `topology`, `activity`, and prepared `workStateCounts`.
Every connection endpoint must name a handle on its endpoint node with the
matching source or target role. Invalid endpoints, inconsistent ticks,
projection issues, invalid counts, and invalid occupancy references render a
contained failure region and may be observed through `onError`. Unexpected
projection, layout, React Flow, and React rendering failures are caught by the
same region-local boundary. Each `FactoryVisualizerError` has a supported
`kind`, a fixed safe `message`, a safe cause name, and `recoverable`; raw thrown
messages, projection identifiers, and payload data are not exposed. A failure
is reported at most once until controlled input recovers or is replaced.

All visible and accessible copy, including state and retry labels, is supplied
through `messages`. Number and tick
formatting uses the optional `formatNumber` callback. Zero Work-State counts are
rendered when supplied; omitting a count omits that node's count presentation.
