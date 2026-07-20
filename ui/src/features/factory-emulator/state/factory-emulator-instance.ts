import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import {
  createFactoryEmulatorSession,
  type FactoryEmulatorCommand,
  type FactoryEmulatorLimits,
  type FactoryEmulatorScenario,
  type FactoryEmulatorSession,
  type FactoryEmulatorSessionStatus,
  type FactoryEventSink,
} from "@you-agent-factory/factory-emulator";
import {
  createFactoryReplayWorldCheckpoint,
  type FactoryReplayWorldCheckpoint,
  type FactoryReplayWorldReducer,
  projectFactoryWorldAtTick,
} from "@you-agent-factory/factory-replay";
import { createStore, type StoreApi } from "zustand/vanilla";

import {
  createFactoryEmulatorSubmissionCommands,
  type FactoryEmulatorSubmissionCommands,
  type FactoryEmulatorSubmissionStoreState,
} from "./factory-emulator-submission";

export interface FactoryEmulatorAdapterError {
  readonly command?: FactoryEmulatorCommand;
  readonly kind: "event-sink-rejected" | "submission-rejected";
  readonly message: string;
  readonly recoverable: true;
}

export interface FactoryEmulatorReplayProjection<State, World> {
  readonly checkpoint: FactoryReplayWorldCheckpoint<State>;
  readonly state: State;
  readonly world: World;
}

export const FACTORY_EMULATOR_PLAYBACK_SPEEDS = [0.5, 1, 2, 4] as const;

export type FactoryEmulatorPlaybackSpeed =
  (typeof FACTORY_EMULATOR_PLAYBACK_SPEEDS)[number];

export interface FactoryEmulatorPlaybackState {
  readonly status: "paused" | "playing";
  readonly speed: FactoryEmulatorPlaybackSpeed;
}

export type FactoryEmulatorAdapterCommand =
  | "followCurrent"
  | "pause"
  | "play"
  | "restart"
  | "retry"
  | "selectTick"
  | "setSpeed"
  | "submit"
  | "step";

export type FactoryEmulatorCommandOutcome =
  | { readonly status: "accepted" }
  | {
      readonly command: FactoryEmulatorAdapterCommand;
      readonly reason: string;
      readonly status: "disabled";
    };

export interface FactoryEmulatorInstanceState<State, World> {
  readonly commandState: "idle" | "running";
  readonly error?: FactoryEmulatorAdapterError;
  readonly events: readonly FactoryEvent[];
  readonly latestTick: number;
  readonly mode: "current" | "history";
  readonly playback: FactoryEmulatorPlaybackState;
  readonly replay: FactoryEmulatorReplayProjection<State, World>;
  readonly selectedTick: number;
  readonly sessionLifecycle: "closed" | "pre-start" | "started";
  readonly sessionStatus: FactoryEmulatorSessionStatus;
  readonly submission: FactoryEmulatorSubmissionStoreState;
}

export interface FactoryEmulatorInstanceOptions<State, World> {
  readonly cloneState: (state: State) => State;
  readonly factory: FactoryDefinition;
  readonly limits?: FactoryEmulatorLimits;
  readonly locale?: string;
  /** Optional caller-owned atomic acceptance boundary, useful for backpressure. */
  readonly beforeCommit?: (events: readonly FactoryEvent[]) => Promise<void>;
  readonly reducer: FactoryReplayWorldReducer<State, World>;
  readonly scenario: FactoryEmulatorScenario;
  readonly yieldControl?: () => void | PromiseLike<void>;
}

export interface FactoryEmulatorInstanceCommands
  extends FactoryEmulatorSubmissionCommands {
  followCurrent(): FactoryEmulatorCommandOutcome;
  pause(): FactoryEmulatorCommandOutcome;
  play(): FactoryEmulatorCommandOutcome;
  restart(): Promise<FactoryEmulatorCommandOutcome>;
  retry(): Promise<FactoryEmulatorCommandOutcome>;
  selectTick(tick: number): FactoryEmulatorCommandOutcome;
  setSpeed(speed: FactoryEmulatorPlaybackSpeed): FactoryEmulatorCommandOutcome;
  start(): Promise<void>;
  step(): Promise<FactoryEmulatorCommandOutcome>;
}

export interface FactoryEmulatorInstance<State, World> {
  readonly commands: FactoryEmulatorInstanceCommands;
  readonly sink: FactoryEventSink;
  readonly store: StoreApi<FactoryEmulatorInstanceState<State, World>>;
}

function rejectionMessage(rejection: unknown): string {
  return rejection instanceof Error ? rejection.message : String(rejection);
}

function cloneEvents(events: readonly FactoryEvent[]): FactoryEvent[] {
  return structuredClone(events) as FactoryEvent[];
}

function accepted(): FactoryEmulatorCommandOutcome {
  return { status: "accepted" };
}

function disabled(
  command: FactoryEmulatorAdapterCommand,
  reason: string,
): FactoryEmulatorCommandOutcome {
  return { command, reason, status: "disabled" };
}

interface CommandRuntime {
  current?: FactoryEmulatorCommand;
  retry?: () => Promise<unknown>;
}

type InstanceStore<State, World> = StoreApi<
  FactoryEmulatorInstanceState<State, World>
>;

type RunEmulatorCommand = (
  command: Exclude<FactoryEmulatorCommand, "reset">,
  invoke: () => Promise<unknown>,
) => Promise<void>;

function replayAtTick<State, World>(
  options: FactoryEmulatorInstanceOptions<State, World>,
  events: readonly FactoryEvent[],
  tick: number,
): FactoryEmulatorReplayProjection<State, World> {
  const cloneState = options.cloneState;
  const result = projectFactoryWorldAtTick({
    events,
    reducer: options.reducer,
    tick,
  });
  return {
    checkpoint: createFactoryReplayWorldCheckpoint(result, cloneState),
    state: cloneState(result.state),
    world: structuredClone(result.world),
  };
}

function createAtomicSink<State, World>(
  options: FactoryEmulatorInstanceOptions<State, World>,
  getStore: () => InstanceStore<State, World>,
  runtime: CommandRuntime,
): FactoryEventSink {
  let sinkTail: Promise<void> = Promise.resolve();
  return {
    write: (batch) => {
      const acceptedBatch = cloneEvents(batch);
      const command = runtime.current;
      const write = sinkTail.then(async () => {
        const store = getStore();
        try {
          await options.beforeCommit?.(cloneEvents(acceptedBatch));
          const events = cloneEvents([
            ...store.getState().events,
            ...acceptedBatch,
          ]);
          const latestTick = events.reduce(
            (latest, event) => Math.max(latest, event.context.tick),
            0,
          );
          const head = projectFactoryWorldAtTick({
            events,
            reducer: options.reducer,
            tick: latestTick,
          });
          const previous = store.getState();
          const selectedTick =
            previous.mode === "current"
              ? head.latestTick
              : previous.selectedTick;
          store.setState({
            error: undefined,
            events: cloneEvents(head.events),
            latestTick: head.latestTick,
            replay:
              selectedTick === head.latestTick
                ? replayAtTick(options, head.events, head.latestTick)
                : replayAtTick(options, head.events, selectedTick),
            selectedTick,
          });
        } catch (rejection) {
          store.setState({
            error: {
              command,
              kind: "event-sink-rejected",
              message: rejectionMessage(rejection),
              recoverable: true,
            },
          });
          throw rejection;
        }
      });
      sinkTail = write.catch(() => undefined);
      return write;
    },
  };
}

function createCommandRunner<State, World>(
  session: FactoryEmulatorSession,
  store: InstanceStore<State, World>,
  runtime: CommandRuntime,
): RunEmulatorCommand {
  return async (command, invoke) => {
    runtime.current = command;
    store.setState({ commandState: "running" });
    try {
      await invoke();
      runtime.retry = undefined;
    } catch (rejection) {
      runtime.retry = invoke;
      throw rejection;
    } finally {
      runtime.current = undefined;
      store.setState({
        commandState: "idle",
        sessionLifecycle: session.state().lifecycle,
        sessionStatus: session.status(),
      });
    }
  };
}

function createPresentationCommands<State, World>(
  options: FactoryEmulatorInstanceOptions<State, World>,
  store: InstanceStore<State, World>,
): Pick<
  FactoryEmulatorInstanceCommands,
  "followCurrent" | "pause" | "play" | "selectTick" | "setSpeed"
> {
  return {
    followCurrent: () => {
      const state = store.getState();
      store.setState({
        mode: "current",
        replay: replayAtTick(options, state.events, state.latestTick),
        selectedTick: state.latestTick,
      });
      return accepted();
    },
    pause: () => {
      const { playback } = store.getState();
      store.setState({ playback: { ...playback, status: "paused" } });
      return accepted();
    },
    play: () => {
      const state = store.getState();
      if (state.mode === "history") {
        return disabled("play", "Return to the current tick before playing.");
      }
      if (state.commandState === "running") {
        return disabled("play", "An emulator command is already running.");
      }
      if (state.sessionStatus.phase === "closed") {
        return disabled("play", "The emulator session is closed.");
      }
      if (state.sessionStatus.phase === "error") {
        return disabled("play", "Retry the failed emulator command first.");
      }
      store.setState({ playback: { ...state.playback, status: "playing" } });
      return accepted();
    },
    selectTick: (tick) => {
      const state = store.getState();
      if (!Number.isSafeInteger(tick) || tick < 0 || tick > state.latestTick) {
        return disabled(
          "selectTick",
          `Select a logical tick from 0 through ${state.latestTick}.`,
        );
      }
      const history = tick < state.latestTick;
      store.setState({
        mode: history ? "history" : "current",
        playback: history
          ? { ...state.playback, status: "paused" }
          : state.playback,
        replay: replayAtTick(options, state.events, tick),
        selectedTick: tick,
      });
      return accepted();
    },
    setSpeed: (speed) => {
      if (!FACTORY_EMULATOR_PLAYBACK_SPEEDS.includes(speed)) {
        return disabled("setSpeed", "Select a supported playback speed.");
      }
      const { playback } = store.getState();
      store.setState({ playback: { ...playback, speed } });
      return accepted();
    },
  };
}

function createExecutionCommands<State, World>(
  options: FactoryEmulatorInstanceOptions<State, World>,
  session: FactoryEmulatorSession,
  store: InstanceStore<State, World>,
  runtime: CommandRuntime,
  run: RunEmulatorCommand,
): Pick<
  FactoryEmulatorInstanceCommands,
  "restart" | "retry" | "start" | "step"
> {
  return {
    restart: async () => {
      const state = store.getState();
      if (state.commandState === "running") {
        return disabled("restart", "An emulator command is already running.");
      }
      if (state.sessionStatus.phase === "closed") {
        return disabled("restart", "The emulator session is closed.");
      }
      session.reset();
      runtime.retry = undefined;
      store.setState({
        commandState: "idle",
        error: undefined,
        events: [],
        latestTick: 0,
        mode: "current",
        playback: { speed: 1, status: "paused" },
        replay: replayAtTick(options, [], 0),
        selectedTick: 0,
        sessionLifecycle: session.state().lifecycle,
        sessionStatus: session.status(),
        submission: { draft: "", nextOrdinal: 1, status: "idle" },
      });
      await run("start", () => session.start());
      return accepted();
    },
    retry: async () => {
      if (runtime.retry === undefined) {
        return disabled("retry", "There is no failed command to retry.");
      }
      const pending = session.status().pendingTransaction;
      if (pending === undefined || pending.command === "reset") {
        return disabled("retry", "There is no retryable emulator transaction.");
      }
      await run(pending.command, runtime.retry);
      return accepted();
    },
    start: () => run("start", () => session.start()),
    step: async () => {
      const state = store.getState();
      if (state.mode === "history") {
        return disabled("step", "Return to the current tick before stepping.");
      }
      if (state.commandState === "running") {
        return disabled("step", "An emulator command is already running.");
      }
      if (state.sessionStatus.phase === "closed") {
        return disabled("step", "The emulator session is closed.");
      }
      if (state.sessionStatus.phase === "error") {
        return disabled("step", "Retry the failed emulator command first.");
      }
      if (session.state().lifecycle !== "started") {
        return disabled("step", "Start the emulator before stepping.");
      }
      await run("advanceToNext", () => session.advanceToNext());
      return accepted();
    },
  };
}

/**
 * Create one website-local emulator boundary. Nothing is shared between calls:
 * the session, event sink, retained history, replay state, and commands all
 * belong to the returned instance.
 */
export function createFactoryEmulatorInstance<State, World>(
  options: FactoryEmulatorInstanceOptions<State, World>,
): FactoryEmulatorInstance<State, World> {
  const runtime: CommandRuntime = {};
  let store: StoreApi<FactoryEmulatorInstanceState<State, World>>;
  const sink = createAtomicSink(options, () => store, runtime);
  const session = createFactoryEmulatorSession({
    factory: options.factory,
    scenario: options.scenario,
    sink,
    ...(options.limits === undefined ? {} : { limits: options.limits }),
    ...(options.yieldControl === undefined
      ? {}
      : { yieldControl: options.yieldControl }),
  });
  store = createStore<FactoryEmulatorInstanceState<State, World>>(() => ({
    commandState: "idle",
    events: [],
    latestTick: 0,
    mode: "current",
    playback: { speed: 1, status: "paused" },
    replay: replayAtTick(options, [], 0),
    selectedTick: 0,
    sessionLifecycle: session.state().lifecycle,
    sessionStatus: session.status(),
    submission: { draft: "", nextOrdinal: 1, status: "idle" },
  }));
  const run = createCommandRunner(session, store, runtime);
  return {
    commands: {
      ...createPresentationCommands(options, store),
      ...createExecutionCommands(options, session, store, runtime, run),
      ...createFactoryEmulatorSubmissionCommands(
        options.factory,
        session,
        store,
        run,
        options.locale,
      ),
    },
    sink,
    store,
  };
}

export const selectFactoryEmulatorEvents = <State, World>(
  state: FactoryEmulatorInstanceState<State, World>,
) => state.events;

export const selectFactoryEmulatorReplay = <State, World>(
  state: FactoryEmulatorInstanceState<State, World>,
) => state.replay;

export const selectFactoryEmulatorError = <State, World>(
  state: FactoryEmulatorInstanceState<State, World>,
) => state.error;
