import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import {
  createFactoryEmulatorSession,
  type FactoryEmulatorCommand,
  type FactoryEmulatorLimits,
  type FactoryEmulatorScenario,
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

export interface FactoryEmulatorAdapterError {
  readonly command?: FactoryEmulatorCommand;
  readonly kind: "event-sink-rejected";
  readonly message: string;
  readonly recoverable: true;
}

export interface FactoryEmulatorReplayProjection<State, World> {
  readonly checkpoint: FactoryReplayWorldCheckpoint<State>;
  readonly state: State;
  readonly world: World;
}

export interface FactoryEmulatorInstanceState<State, World> {
  readonly commandState: "idle" | "running";
  readonly error?: FactoryEmulatorAdapterError;
  readonly events: readonly FactoryEvent[];
  readonly latestTick: number;
  readonly mode: "current";
  readonly replay: FactoryEmulatorReplayProjection<State, World>;
  readonly selectedTick: number;
  readonly sessionStatus: FactoryEmulatorSessionStatus;
}

export interface FactoryEmulatorInstanceOptions<State, World> {
  readonly cloneState: (state: State) => State;
  readonly factory: FactoryDefinition;
  readonly limits?: FactoryEmulatorLimits;
  /** Optional caller-owned atomic acceptance boundary, useful for backpressure. */
  readonly beforeCommit?: (events: readonly FactoryEvent[]) => Promise<void>;
  readonly reducer: FactoryReplayWorldReducer<State, World>;
  readonly scenario: FactoryEmulatorScenario;
  readonly yieldControl?: () => void | PromiseLike<void>;
}

export interface FactoryEmulatorInstanceCommands {
  retry(): Promise<void>;
  start(): Promise<void>;
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

/**
 * Create one website-local emulator boundary. Nothing is shared between calls:
 * the session, event sink, retained history, replay state, and commands all
 * belong to the returned instance.
 */
export function createFactoryEmulatorInstance<State, World>(
  options: FactoryEmulatorInstanceOptions<State, World>,
): FactoryEmulatorInstance<State, World> {
  const initialResult = projectFactoryWorldAtTick({
    events: [],
    reducer: options.reducer,
    tick: 0,
  });
  let currentCommand: FactoryEmulatorCommand | undefined;
  let retryPending: (() => Promise<unknown>) | undefined;
  let sinkTail: Promise<void> = Promise.resolve();
  let store: StoreApi<FactoryEmulatorInstanceState<State, World>>;

  const sink: FactoryEventSink = {
    write: (batch) => {
      const acceptedBatch = cloneEvents(batch);
      const command = currentCommand;
      const write = sinkTail.then(async () => {
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
          const result = projectFactoryWorldAtTick({
            events,
            reducer: options.reducer,
            tick: latestTick,
          });
          store.setState({
            error: undefined,
            events: cloneEvents(result.events),
            latestTick: result.latestTick,
            replay: {
              checkpoint: createFactoryReplayWorldCheckpoint(
                result,
                options.cloneState,
              ),
              state: options.cloneState(result.state),
              world: structuredClone(result.world),
            },
            selectedTick: result.selectedTick,
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
    replay: {
      checkpoint: createFactoryReplayWorldCheckpoint(
        initialResult,
        options.cloneState,
      ),
      state: options.cloneState(initialResult.state),
      world: structuredClone(initialResult.world),
    },
    selectedTick: 0,
    sessionStatus: session.status(),
  }));

  const run = async (
    command: Exclude<FactoryEmulatorCommand, "reset">,
    invoke: () => Promise<unknown>,
  ): Promise<void> => {
    currentCommand = command;
    store.setState({ commandState: "running" });
    try {
      await invoke();
      retryPending = undefined;
    } catch (rejection) {
      retryPending = invoke;
      throw rejection;
    } finally {
      currentCommand = undefined;
      store.setState({
        commandState: "idle",
        sessionStatus: session.status(),
      });
    }
  };

  return {
    commands: {
      retry: async () => {
        if (retryPending === undefined) return;
        const pending = session.status().pendingTransaction;
        if (pending === undefined || pending.command === "reset") return;
        await run(pending.command, retryPending);
      },
      start: () => run("start", () => session.start()),
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

export const selectFactoryEmulatorSessionStatus = <State, World>(
  state: FactoryEmulatorInstanceState<State, World>,
) => state.sessionStatus;
