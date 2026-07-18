import type { FactoryEvent } from "@you-agent-factory/client";

export type FactoryReplaySelection =
  | { mode: "current" }
  | { mode: "fixed"; tick: number };

export interface FactoryReplayReducer<State, World> {
  createState(selectedTick: number): State;
  applyEvent(state: State, event: FactoryEvent): State;
  projectWorld(state: State): World;
}

export interface FactoryReplayInitialization<State, World> {
  events: readonly FactoryEvent[];
  reducer: FactoryReplayReducer<State, World>;
  selection: FactoryReplaySelection;
}

export interface FactoryReplayResult<State, World> {
  appliedEvents: FactoryEvent[];
  events: FactoryEvent[];
  latestTick: number;
  selectedTick: number;
  selection: FactoryReplaySelection;
  state: State;
  world: World;
}

export function compareFactoryEvents(
  left: FactoryEvent,
  right: FactoryEvent,
): number;

export function canonicalizeFactoryEvents(
  events: readonly FactoryEvent[],
): FactoryEvent[];

export function initializeFactoryReplay<State, World>(
  input: FactoryReplayInitialization<State, World>,
): FactoryReplayResult<State, World>;

export function projectFactoryWorldAtTick<State, World>(
  input: Omit<FactoryReplayInitialization<State, World>, "selection"> & {
    tick: number;
  },
): FactoryReplayResult<State, World>;
