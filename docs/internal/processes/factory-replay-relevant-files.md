# Factory Replay Relevant Files

The canonical public replay implementation lives under
`ui/packages/factory-replay`. The retired root `packages/factory-replay`
implementation must not be recreated.

- `src/replay.ts` owns event ordering, checkpoint creation, accepted-tail
  replay, and selected-tick world reconstruction.
- `src/topology.ts`, `src/activity.ts`, `src/load.ts`, and `src/progress.ts` own
  disposable projections derived from canonical Factory events.
- `src/index.ts` is the only public export boundary.
- Package-local tests cover ordering, checkpoints, missing data, topology,
  activity, load, and mutually exclusive Work progress classification.
- `scripts/check-package-boundary.mjs` keeps replay independent of React,
  dashboard state, persistence, transport, and the emulator.
- `scripts/build-package.mjs`, `verify-package-pack.mjs`, and
  `verify-installed-consumer.mjs` prove the published `dist` surface.

Run the package verification directly with:

```sh
cd ui/packages/factory-replay
bun run verify
```

Run the complete frontend package release family with:

```sh
make ui-public-package-release
```
