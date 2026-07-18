import type { FactoryEvent } from "@you-agent-factory/client";
import type { FactoryDefinition } from "@you-agent-factory/client";

export type FactoryTopologyNodeKind =
  | "resource"
  | "worker"
  | "work-state"
  | "work-type"
  | "workstation";

export type FactoryTopologyConnectionKind =
  | "worker-assignment"
  | "worker-resource"
  | "workstation-input"
  | "workstation-on-continue"
  | "workstation-on-failure"
  | "workstation-on-rejection"
  | "workstation-output"
  | "workstation-resource"
  | "work-type-state";

export interface FactoryTopologyHandle {
  id: string;
  role: "source" | "target";
}

interface FactoryTopologyNodeBase {
  entityId: string;
  handles: FactoryTopologyHandle[];
  id: string;
  kind: FactoryTopologyNodeKind;
  label: string;
}

export interface FactoryResourceTopologyNode extends FactoryTopologyNodeBase {
  capacity: number;
  kind: "resource";
}

export interface FactoryWorkerTopologyNode extends FactoryTopologyNodeBase {
  kind: "worker";
}

export interface FactoryWorkTypeTopologyNode extends FactoryTopologyNodeBase {
  kind: "work-type";
}

export interface FactoryWorkStateTopologyNode extends FactoryTopologyNodeBase {
  category: NonNullable<FactoryDefinition["workTypes"]>[number]["states"][number]["type"];
  kind: "work-state";
  workTypeId: string;
}

export interface FactoryWorkstationTopologyNode
  extends FactoryTopologyNodeBase {
  kind: "workstation";
}

export type FactoryTopologyNode =
  | FactoryResourceTopologyNode
  | FactoryWorkerTopologyNode
  | FactoryWorkStateTopologyNode
  | FactoryWorkTypeTopologyNode
  | FactoryWorkstationTopologyNode;

export interface FactoryTopologyConnectionEndpoint {
  handleId: string;
  nodeId: string;
}

export interface FactoryTopologyConnection {
  id: string;
  kind: FactoryTopologyConnectionKind;
  source: FactoryTopologyConnectionEndpoint;
  target: FactoryTopologyConnectionEndpoint;
}

export interface FactoryTopologyProjectionIssue {
  code: "DUPLICATE_ENTITY_ID" | "MISSING_FACTORY" | "UNRESOLVED_CONNECTION";
  connectionKind?: FactoryTopologyConnectionKind;
  id: string;
  message: string;
  nodeId?: string;
  sourceReference?: string;
  targetReference?: string;
}

export interface FactoryTopologyProjection {
  connections: FactoryTopologyConnection[];
  issues: FactoryTopologyProjectionIssue[];
  nodes: FactoryTopologyNode[];
  selectedTick: number;
}

export interface FactoryTopologyProjectionInput {
  factory?: FactoryDefinition | undefined;
  selectedTick: number;
}

export interface FactoryTopologyAtTickInput {
  events: readonly FactoryEvent[];
  tick: number;
}

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

export function projectFactoryTopology(
  input: FactoryTopologyProjectionInput,
): FactoryTopologyProjection;

export function projectFactoryTopologyAtTick(
  input: FactoryTopologyAtTickInput,
): FactoryTopologyProjection;
