# `@you-agent-factory/factory-visualizers`

Controlled React components for rendering caller-prepared Factory replay
projections. The package exports `FactoryTopologyReplay`,
`FactoryRecordingTopologyReplay`, `FactoryTimelineScrubber`, and
`WorkProgressVisualizer` together with their message, formatting, status,
callback, and structured error contracts.

The host always owns transport, persistence, and canonical Factory data. It can
either prepare controlled projections with `@you-agent-factory/factory-replay`
or pass an unknown recording directly to `FactoryRecordingTopologyReplay`,
which validates it through `@you-agent-factory/client` before deriving the
selected-tick projections. Import the package styles once:

```tsx
import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayProps,
} from "@you-agent-factory/factory-visualizers";
import "@you-agent-factory/components/styles.css";
import "@you-agent-factory/factory-visualizers/styles.css";

export function Topology(props: FactoryTopologyReplayProps) {
  return <FactoryTopologyReplay {...props} />;
}
```

`FactoryRecordingTopologyReplay` reports invalid input as one sanitized
`recording-validation` diagnostic and renders the same accessible failed
presentation as the controlled topology component. It does not require a
router, data provider, store, browser persistence, network request, or backend.

Recording playback opens in current mode at the latest accepted logical tick.
Pass a `defaultSelectedTick` that exists in the recording to open a fixed
historical projection instead. The shared timeline selects only recorded ticks,
keeps fixed history stable as later evidence arrives, and can return to current
mode with its follow-latest action. Current mode also incorporates newly
accepted same-tick events in canonical sequence order.

Run `bun run verify` in this directory to typecheck and test the components,
build Storybook, exercise accessibility and responsive behavior, validate the
compiled dependency boundary and tarball inventory, and install, build, and
render all public components in a clean temporary consumer.
