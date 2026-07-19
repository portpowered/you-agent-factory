# `@you-agent-factory/factory-visualizers`

Controlled React components for rendering caller-prepared Factory replay
projections. The package exports `FactoryTopologyReplay`,
`FactoryTimelineScrubber`, and `WorkProgressVisualizer` together with their
message, formatting, status, callback, and structured error contracts.

The host owns event replay, transport, persistence, playback, selected state,
and canonical Factory data. Import the package styles once, prepare projections
with `@you-agent-factory/factory-replay`, and pass controlled props:

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

Run `bun run verify` in this directory to typecheck and test the components,
build Storybook, exercise accessibility and responsive behavior, validate the
compiled dependency boundary and tarball inventory, and install, build, and
render all public components in a clean temporary consumer.
