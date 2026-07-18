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

export interface FactoryTopologyReplayHandle {
  id: string;
  role: "source" | "target";
}

export interface FactoryTopologyReplayNode {
  category?: string;
  capacity?: number;
  entityId: string;
  handles: readonly FactoryTopologyReplayHandle[];
  id: string;
  kind: FactoryTopologyNodeKind;
  label: string;
  workTypeId?: string;
}

export interface FactoryTopologyReplayConnectionEndpoint {
  handleId: string;
  nodeId: string;
}

export interface FactoryTopologyReplayConnection {
  id: string;
  kind: FactoryTopologyConnectionKind;
  source: FactoryTopologyReplayConnectionEndpoint;
  target: FactoryTopologyReplayConnectionEndpoint;
}

export interface FactoryTopologyReplayTopology {
  connections: readonly FactoryTopologyReplayConnection[];
  issues: readonly { code: string; id: string; message: string }[];
  nodes: readonly FactoryTopologyReplayNode[];
  selectedTick: number;
}

export interface FactoryTopologyReplayDispatch {
  id: string;
  resourceIds: readonly string[];
  startedTick: number;
  transitionId: string;
  workIds: readonly string[];
  workerId?: string;
  workstationId?: string;
  workstationNodeId?: string;
}

export interface FactoryTopologyReplayOccupancy {
  availableQuantity?: number;
  capacity: number;
  evidence: "known" | "unavailable";
  occupiedQuantity?: number;
  occupiedTokenIds?: readonly string[];
  resourceId: string;
  resourceNodeId: string;
}

export interface FactoryTopologyReplayActivity {
  activeDispatches: readonly FactoryTopologyReplayDispatch[];
  activeWorkstationIds: readonly string[];
  issues: readonly { code: string; id: string; message: string }[];
  resourceOccupancy: readonly FactoryTopologyReplayOccupancy[];
  selectedTick: number;
}

export interface FactoryTopologyReplayWorkStateCount {
  count: number;
  nodeId: string;
}

export interface FactoryTopologyReplayProjection {
  activity: FactoryTopologyReplayActivity;
  topology: FactoryTopologyReplayTopology;
  workStateCounts: readonly FactoryTopologyReplayWorkStateCount[];
}

export type FactoryVisualizerErrorKind =
  | "endpoint"
  | "layout"
  | "projection"
  | "rendering";

export interface FactoryVisualizerError {
  cause?: unknown;
  kind: FactoryVisualizerErrorKind;
  message: string;
  recoverable: boolean;
}

export interface FactoryTopologyReplayMessages {
  activeDispatchCount: (count: number) => string;
  connectionLabel: (kind: FactoryTopologyConnectionKind) => string;
  failedDescription: string;
  failedTitle: string;
  handleLabel: (handleId: string, role: "source" | "target") => string;
  inactiveDispatch: string;
  nodeKind: (kind: FactoryTopologyNodeKind) => string;
  occupancy: (occupied: string, capacity: string) => string;
  occupancyUnavailable: string;
  regionLabel: string;
  selectedNode: string;
  selectedTick: (tick: string) => string;
  workStateCount: (count: string) => string;
}
