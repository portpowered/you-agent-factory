# `@you-agent-factory/factory-visualizers`

Controlled React components for rendering caller-prepared Factory replay
projections. The package exports `FactoryTopologyReplay`,
`FactoryRecordingTopologyReplay`, `FactoryTimelineScrubber`, and
`WorkProgressVisualizer`, and `FactoryEmulatorControls` together with their
message, formatting, status, callback, and structured error contracts.

`FactoryEmulatorControls` composes the lower-level controlled playback toolbar
with `FactoryTimelineScrubber`. Hosts provide the current/history selection and
all callbacks. Selecting an earlier tick requests pause before selection; Play
and Step request follow-latest before their host command in history mode. It
never creates a timer, mutates replay data, or owns emulator state.

`FactoryEmulatorView` vertically composes host-supplied controls, topology,
Work progress, and an optional submission region. Its `full` preset shows every
region; `compact` omits speed and submission; `display-only` renders topology
only. Pass `visibility` to override any individual playback, timeline, speed,
runtime-status, progress, or submission region. Hidden regions are not rendered.

Both emulator exports accept `onError` for sanitized structured render or
composition diagnostics and an optional `failure` for a safe host-provided
local failure message. A failure may include `recoveryAction` with a host-owned
callback; the visualizers render that action but never assume a retry,
transport, timer, or global error-state implementation.

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

The package also ships a validated recording that can be imported through the
intentional example export:

```tsx
import supportPlayback from "@you-agent-factory/factory-visualizers/examples/support-playback.factory-recording.v1.json";

<FactoryRecordingTopologyReplay
  defaultSelectedTick={1}
  messages={messages}
  recording={supportPlayback}
/>;
```

`FactoryRecordingTopologyReplay` reports invalid input as one sanitized
`recording-validation` diagnostic and renders the same accessible failed
presentation as the controlled topology component. It does not require a
router, data provider, store, browser persistence, network request, or backend.
Asynchronous hosts can pass a controlled `state` with `loading`, `ready`, or
`failed` status; a controlled state takes precedence over the direct `recording`
shorthand. Controlled failures report their structured visualizer diagnostic
once while keeping diagnostic details out of the localized presentation.

A validated recording whose selected projection has no topology nodes renders
the shared empty presentation. Loading, empty, validation failure, and
projection failure therefore remain distinct from a successfully rendered
closed local recording.

Recording playback opens in current mode at the latest accepted logical tick.
Pass a `defaultSelectedTick` that exists in the recording to open a fixed
historical projection instead. The shared timeline selects only recorded ticks,
keeps fixed history stable as later evidence arrives, and can return to current
mode with its follow-latest action. Current mode also incorporates newly
accepted same-tick events in canonical sequence order.

Run `make storybook` in this directory for package-local development. Use
`make storybook-build` and `make storybook-test` for deterministic static and
browser verification, or run `bun run verify` to typecheck, lint, and test the
components, exercise Storybook accessibility and responsive behavior, validate
the compiled dependency boundary and tarball inventory, and install, build,
and render the public recording composition in a clean temporary consumer.
