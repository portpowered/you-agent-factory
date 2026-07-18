import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";

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
  category: NonNullable<
    FactoryDefinition["workTypes"]
  >[number]["states"][number]["type"];
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
  factory?: FactoryDefinition;
  selectedTick: number;
}

export interface FactoryTopologyAtTickInput {
  events: readonly FactoryEvent[];
  tick: number;
}
