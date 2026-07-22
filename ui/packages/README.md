# Public frontend package family

The public frontend packages let a TypeScript or React host validate canonical
Factory data, replay recordings, run a deterministic browser subset, and render
read-only Factory views. They do not move execution authority out of the hosted
Go runtime.

## Package roles and dependency direction

| Layer | Package or contract | Responsibility |
| --- | --- | --- |
| API contracts | Generated contract types exposed by `@you-agent-factory/client` | Canonical `FactoryDefinition`, `FactoryEvent`, and OpenAPI `components`, `paths`, and `operations` types. These are the shared data boundary, not an execution runtime. |
| Client | `@you-agent-factory/client` | Transport-neutral validation and parsing for Factory definitions, canonical events, recordings, replay text, and presentation-only layout sidecars. It does not open a network connection. |
| Replay | `@you-agent-factory/factory-replay` | Pure, framework-independent ordering, accepted-event checkpoints, selected-tick world reconstruction, and topology, activity, load, and Work-progress projections. It consumes client contracts as types and owns no history or persistence. |
| Emulator | `@you-agent-factory/factory-emulator` | Deterministic, transport-neutral execution of the documented Petri subset into a caller-owned canonical-event sink. It uses client contracts but is independent of React, replay, visualizers, browser timers, and storage. |
| Components | `@you-agent-factory/components` | Domain-neutral React controls, graph primitives, styles, and design tokens. Hosts supply state and callbacks. |
| Visualizers | `@you-agent-factory/factory-visualizers` | Controlled, read-only Factory replay and emulator compositions. It combines client/replay contracts with components but owns no transport, execution, playback timer, durable state, or dashboard store. |

The intended direction is contracts/client to replay or emulator, then
components plus replay into visualizers, and finally into a consumer host. The
emulator does not depend on replay or visualizers. Package code never imports
the hosted dashboard to obtain state or behavior.

## Verify the complete package family

From a fresh checkout with Bun 1.3.12 available, run the UI-owned aggregate
release gate from the repository root:

```sh
cd ui
bun run verify:public-packages
```

The command installs the locked UI toolchain once, then runs each package's
existing release gate in dependency-safe order: client, components, replay,
emulator, then visualizers. Those package gates retain their own pack and clean
installed-consumer checks. Immediately after the replay gate, the aggregate
command also runs the established deterministic 10,000-event replay regression,
including its retained-memory bound. After building client and replay, the
command links those workspace packages into the installed UI toolchain for
downstream local type resolution without copying source package trees. After
the five package gates, it runs the focused production website adapter and
customer-demo state/component regressions, builds their browser acceptance
stories, and verifies the adapter and both demos at desktop and narrow
viewports, including reduced-motion behavior. It then runs the focused hosted
topology adapter/component regressions and verifies exact-session replay in the
browser at desktop and narrow viewports, including same-tick event ordering,
session switching, refresh isolation, and explicit loading, empty, error, and
success outcomes. The aggregate command stops at the first failed install,
behavioral regression, build, browser check, link, or release gate and reports
the package family or package, step, command, and command outcome. It does not
depend on a root Makefile release target.

## Hosted execution and browser emulation

Hosted Factory Sessions executed by the Go runtime are authoritative. They own
the complete orchestration and provider behavior, append the canonical event
stream, and remain the source for live and historical dashboard state.

The emulator is a deterministic browser-friendly subset for examples, tests,
and offline interaction. Its accepted output uses the canonical Factory-event
contracts, so the same replay and visualization layers can consume it. This
compatibility does **not** make the emulator a replacement for hosted execution
or a promise that every executable Factory has browser semantics. Always call
`inspectFactoryEmulatorCompatibility` or parse through the emulator boundary
before starting a session.

Emulator v1 intentionally does not provide:

- JavaScript orchestration or arbitrary JavaScript execution;
- Web Workers, wall-clock scheduling, autoplay, or host task policy;
- Factory or graph editing, save behavior, or asset fetching/resolution;
- dependency visualization, dependency-specific controls, or editable
  dependency graphs (supported `DEPENDS_ON` scheduling remains execution-only);
- browser persistence, hosted Factory Sessions, SSE, networking, or reconnects;
- real providers, model calls, provider-global quotas, or provider sessions;
- runtime Work spawning, parent/peer/spawn-aware guards, or relationship types
  other than submission-time `DEPENDS_ON`;
- Factory-level and `AGENT_RUN` guards, classifiers, unsupported workstation or
  worker execution types, or the rest of the hosted executable feature set.

Presentation-only embedded raster annotations in a validated client layout
sidecar are not executable Factory asset references. Visualizers render those
annotations read-only and never fetch an asset URL.

## Install and consume packed exports

Install only the layers used by the host. A React host that provides both
recording and emulator examples can install the complete family:

```sh
npm install @you-agent-factory/client \
  @you-agent-factory/factory-replay \
  @you-agent-factory/factory-emulator \
  @you-agent-factory/components \
  @you-agent-factory/factory-visualizers \
  react react-dom
```

The repository proves the equivalent installation from registry-format
tarballs, rather than workspace source links, with:

```sh
cd ui/packages/factory-visualizers
bun run check:installed-consumer
```

That check installs all five packed packages into a temporary clean project,
typechecks their declarations, creates a production bundle, and renders both
the static and interactive paths in Chromium.

### Static recording

Use only the public example and stylesheet subpaths. The recording component
validates unknown input and contains invalid-recording failures instead of
blanking the host application.

```tsx
import {
  FactoryRecordingTopologyReplay,
  type FactoryRecordingTopologyReplayMessages,
} from "@you-agent-factory/factory-visualizers";
import recording from "@you-agent-factory/factory-visualizers/examples/support-playback.factory-recording.v1.json";
import "@you-agent-factory/components/styles.css";
import "@you-agent-factory/factory-visualizers/styles.css";

export function RecordingExample({
  messages,
}: {
  messages: FactoryRecordingTopologyReplayMessages;
}) {
  return (
    <FactoryRecordingTopologyReplay
      formatNumber={String}
      messages={messages}
      recording={recording}
    />
  );
}
```

`@you-agent-factory/client` also publishes its recording and visualization
layout examples, while `@you-agent-factory/factory-emulator/schema` is the
documented scenario JSON Schema subpath for tooling.

### Interactive emulator

The interactive path imports the published scenario, creates one session and
one sink per mounted example, projects the sink's caller-owned history, and
passes controlled state and actions to `FactoryEmulatorView`:

```ts
import type { FactoryDefinition, FactoryEvent } from "@you-agent-factory/client";
import {
  createFactoryEmulatorSession,
  parseFactoryEmulatorScenario,
  type FactoryEventSink,
} from "@you-agent-factory/factory-emulator";
import scenarioInput from "@you-agent-factory/factory-emulator/examples/customer-support.scenario.v1.json";
import {
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgressAtTick,
} from "@you-agent-factory/factory-replay";

declare const factory: FactoryDefinition;

const history: FactoryEvent[] = [];
const sink: FactoryEventSink = {
  write: async (batch) => {
    history.push(...structuredClone(batch));
  },
};
const scenario = parseFactoryEmulatorScenario(scenarioInput, factory);
const session = createFactoryEmulatorSession({ factory, scenario, sink });

await session.start();
await session.advanceToNext();
const tick = history.at(-1)?.context.tick ?? 0;
const topology = projectFactoryTopologyAtTick({ events: history, tick });
const progress = projectFactoryWorkProgressAtTick({ events: history, tick });
```

Here `factory` is a caller-owned compatible `FactoryDefinition`. A React host
imports `FactoryEmulatorView` from the visualizer root and supplies its own
state, messages, playback policy, projection composition, and controlled props.
The clean installed-consumer check exercises that complete wiring with two
isolated instances.

## Failure, state, and accessibility ownership

- Emulator commands calculate an atomic canonical-event batch, await the
  caller-owned sink, and commit only after acceptance. Sink rejection remains
  retryable; budget or zero-duration-cycle boundaries pause with a structured
  error rather than fabricating terminal Work.
- `reset()` restores deterministic session-owned state and clears an execution
  pause, but retains the sink. The caller must clear or replace its own event
  history before a byte-equivalent rerun.
- Event history, replay checkpoints, selected tick, playback, errors,
  submissions, and persistence are caller-owned. Keep one ownership boundary
  per mounted example; package globals are not an isolation mechanism.
- Visualizers expose labeled regions, named controls, disabled states, visible
  focus, keyboard-operable playback/history controls, status semantics, and
  contained alerts. Hosts must provide meaningful localized messages and keep
  recovery actions operable.
- Hosts that autoplay must observe `prefers-reduced-motion`, start or become
  paused when reduced motion is requested, and preserve manual Pause and
  historical selection until an explicit user action resumes current playback.
  The emulator and visualizers do not create an autoplay timer.

## Project boundary

This package family is frontend-only. Its contracts and canonical-compatible
emulator events reuse the existing public API vocabulary, but adopting these
packages does not imply changes to the backend, authored OpenAPI, generated Go
code, CLI behavior, hosted execution, or functional-test surface.

## Maintainer publication

The five frontend packages are versioned and published as one family. Prepare
registry-format tarballs locally without publishing with:

```sh
make ui-public-package-publish-prepare \
  PACKAGE_VERSION=1.2.3 \
  PACKAGE_OUTPUT=.artifacts/public-packages
```

Pull requests run the same candidate preparation as a dry run. Successful
protected-main builds publish an immutable `0.0.2-dev.<run>.<commit>` family
under the npm `dev` tag. A successful `vX.Y.Z` release-candidate workflow lets
the release workflow publish all five packages at `X.Y.Z` under `latest` using
npm trusted publishing. Internal package dependency and peer-dependency pins
are rewritten only in the staged tarballs so every published family is aligned.
