# Factory emulator controls

`FactoryEmulatorControls` is a controlled presentation component for a Factory
emulator host. It renders playback commands, the four supported speed choices,
and a host-provided runtime status. It does not create timers, call an emulator,
or keep replay or event-history state.

```tsx
<FactoryEmulatorControls
  isPlaying={status === "running"}
  onPause={pause}
  onPlay={play}
  onRestart={restart}
  onSpeedChange={setSpeed}
  onStep={advanceToNextLogicalTick}
  runtimeStatus={{ label: "Ready", tone: "success" }}
  speed={1}
/>
```

`onStep` is always one host command. Speed selection is reported separately by
`onSpeedChange`; hosts decide how speed affects their own timers. Use
`disabledActions` when the host knows an operation is unavailable.
