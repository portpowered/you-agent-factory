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

export type FactoryReplayStateCloner<State> = (state: State) => State;

export interface FactoryReplayCheckpoint<State> {
  acceptedEventIDs: readonly string[];
  selectedTick: number;
  state: State;
}

export interface FactoryReplayAdvanceInput<State, World> {
  checkpoint: FactoryReplayCheckpoint<State>;
  cloneState: FactoryReplayStateCloner<State>;
  events: readonly FactoryEvent[];
  reducer: FactoryReplayReducer<State, World>;
  setSelectedTick(state: State, tick: number): State;
  tick: number;
}

export interface FactoryReplayAdvanceResult<State, World> {
  appliedEvents: FactoryEvent[];
  checkpoint: FactoryReplayCheckpoint<State>;
  latestTick: number;
  selectedTick: number;
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

export function createFactoryReplayCheckpoint<State, World>(
  result: FactoryReplayResult<State, World>,
  cloneState: FactoryReplayStateCloner<State>,
): FactoryReplayCheckpoint<State>;

export function advanceFactoryReplay<State, World>(
  input: FactoryReplayAdvanceInput<State, World>,
): FactoryReplayAdvanceResult<State, World>;

export function projectFactoryWorldAtTick<State, World>(
  input: Omit<FactoryReplayInitialization<State, World>, "selection"> & {
    tick: number;
  },
): FactoryReplayResult<State, World>;
