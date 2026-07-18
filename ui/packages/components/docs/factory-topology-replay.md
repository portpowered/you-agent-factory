# FactoryTopologyReplay

`FactoryTopologyReplay` renders one host-prepared Factory topology and activity
projection. Import it from `@you-agent-factory/components/visualizers` and load
the package `styles.css` entrypoint once in the host.

The host owns transport, raw events, replay, selected ticks, checkpoints,
playback, storage, selection, and Factory mutation. The visualizer owns only a
disposable React Flow projection. Pass a new `projection` to replace the visible
tick and pass `selectedNodeId` back after `onSelectNode` to control selection.

The projection contains `topology`, `activity`, and prepared `workStateCounts`.
Every connection endpoint must name a handle on its endpoint node with the
matching source or target role. Invalid endpoints, inconsistent ticks,
projection issues, invalid counts, and invalid occupancy references render a
contained failure region and may be observed through `onError`; diagnostics do
not render projection identifiers or payload data.

All visible and accessible copy is supplied through `messages`. Number and tick
formatting uses the optional `formatNumber` callback. Zero Work-State counts are
rendered when supplied; omitting a count omits that node's count presentation.
