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

/**
 * The canonical relationship vocabulary used to declare node handles and
 * project connection endpoints. Renderers can use this same value to render
 * handles without maintaining a parallel mapping.
 */
export const FACTORY_TOPOLOGY_RELATIONSHIPS = {
  "worker-assignment": {
    source: { handleId: "worker-assignment-source", nodeKind: "worker" },
    target: {
      handleId: "worker-assignment-target",
      nodeKind: "workstation",
    },
  },
  "worker-resource": {
    source: { handleId: "worker-resource-source", nodeKind: "resource" },
    target: { handleId: "worker-input-target", nodeKind: "worker" },
  },
  "workstation-input": {
    source: { handleId: "workstation-input-source", nodeKind: "work-state" },
    target: {
      handleId: "workstation-input-target",
      nodeKind: "workstation",
    },
  },
  "workstation-on-continue": {
    source: {
      handleId: "workstation-on-continue-source",
      nodeKind: "workstation",
    },
    target: { handleId: "work-state-input-target", nodeKind: "work-state" },
  },
  "workstation-on-failure": {
    source: {
      handleId: "workstation-on-failure-source",
      nodeKind: "workstation",
    },
    target: { handleId: "work-state-input-target", nodeKind: "work-state" },
  },
  "workstation-on-rejection": {
    source: {
      handleId: "workstation-on-rejection-source",
      nodeKind: "workstation",
    },
    target: { handleId: "work-state-input-target", nodeKind: "work-state" },
  },
  "workstation-output": {
    source: {
      handleId: "workstation-output-source",
      nodeKind: "workstation",
    },
    target: { handleId: "work-state-input-target", nodeKind: "work-state" },
  },
  "workstation-resource": {
    source: {
      handleId: "workstation-resource-source",
      nodeKind: "resource",
    },
    target: {
      handleId: "workstation-resource-target",
      nodeKind: "workstation",
    },
  },
  "work-type-state": {
    source: { handleId: "work-type-state-source", nodeKind: "work-type" },
    target: { handleId: "work-type-state-target", nodeKind: "work-state" },
  },
} as const;

export type FactoryTopologyConnectionKind =
  keyof typeof FACTORY_TOPOLOGY_RELATIONSHIPS;

export type FactoryTopologyHandleId =
  (typeof FACTORY_TOPOLOGY_RELATIONSHIPS)[FactoryTopologyConnectionKind][
    | "source"
    | "target"]["handleId"];

export interface FactoryTopologyHandle {
  id: FactoryTopologyHandleId;
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
  code:
    | "DUPLICATE_ENTITY_ID"
    | "INVALID_CONNECTION_ENDPOINT"
    | "MISSING_FACTORY"
    | "UNSUPPORTED_CONNECTION_KIND";
  connectionId?: string;
  connectionKind?: string;
  endpoint?: "source" | "target";
  endpointReason?: "MISSING_HANDLE" | "MISSING_NODE" | "NODE_KIND_MISMATCH";
  expectedNodeKind?: FactoryTopologyNodeKind;
  handleId?: string;
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
  ok: boolean;
  selectedTick: number;
}

export interface FactoryTopologyConnectionCandidate {
  kind: string;
  sourceNodeId?: string;
  sourceReference: string;
  targetNodeId?: string;
  targetReference: string;
}

export type FactoryTopologyConnectionResult =
  | { connection: FactoryTopologyConnection; issue?: never; ok: true }
  | { connection?: never; issue: FactoryTopologyProjectionIssue; ok: false };

export interface FactoryTopologyProjectionInput {
  factory?: FactoryDefinition;
  selectedTick: number;
}

export interface FactoryTopologyAtTickInput {
  events: readonly FactoryEvent[];
  tick: number;
}
