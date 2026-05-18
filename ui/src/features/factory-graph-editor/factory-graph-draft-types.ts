import type { DashboardTopology } from "../../api/dashboard/types";
import type {
  CanonicalFactoryDefinition,
  EditableFactoryDefinitionDocument,
} from "../../api/current-factory-definition";

type FactoryResource = NonNullable<CanonicalFactoryDefinition["resources"]>[number];
type FactoryWorker = NonNullable<CanonicalFactoryDefinition["workers"]>[number];
type FactoryWorkType = NonNullable<CanonicalFactoryDefinition["workTypes"]>[number];
type FactoryWorkState = FactoryWorkType["states"][number];
type FactoryWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type FactoryWorkstationIO = FactoryWorkstation["inputs"][number];

export type { DashboardTopology };
export type {
  CanonicalFactoryDefinition,
  EditableFactoryDefinitionDocument,
  FactoryResource,
  FactoryWorker,
  FactoryWorkState,
  FactoryWorkstation,
  FactoryWorkstationIO,
  FactoryWorkType,
};

export type FactoryGraphNodeKind =
  | "resource"
  | "worker"
  | "work-state"
  | "work-type"
  | "workstation";

export type FactoryGraphEdgeKind =
  | "worker-assignment"
  | "worker-resource"
  | "workstation-input"
  | "workstation-on-continue"
  | "workstation-on-failure"
  | "workstation-on-rejection"
  | "workstation-output"
  | "workstation-resource"
  | "work-type-state";

export interface FactoryGraphNodeReference {
  kind: Exclude<FactoryGraphNodeKind, "work-state">;
  name: string;
}

export interface FactoryGraphWorkStateReference {
  kind: "work-state";
  stateName: string;
  workTypeName: string;
}

export type FactoryGraphNodeKey =
  | FactoryGraphNodeReference
  | FactoryGraphWorkStateReference;

export interface FactoryGraphNode {
  id: string;
  key: FactoryGraphNodeKey;
  kind: FactoryGraphNodeKind;
  label: string;
}

export interface FactoryGraphEdge {
  id: string;
  kind: FactoryGraphEdgeKind;
  source: FactoryGraphNodeKey;
  sourceId: string;
  target: FactoryGraphNodeKey;
  targetId: string;
}

export interface FactoryGraphDraftValidationError {
  code:
    | "DUPLICATE_IDENTIFIER"
    | "INCOMPATIBLE_EDGE"
    | "INVALID_RELATIONSHIP_TYPE"
    | "MISSING_REQUIRED_FIELD"
    | "UNKNOWN_NODE";
  field?: string;
  message: string;
  target:
    | {
        kind: "edge";
        id: string;
      }
    | {
        kind: "field";
        field: string;
      }
    | {
        kind: "node";
        id: string;
      };
}

export interface FactoryGraphDraftEdgeChange {
  kind:
    | "worker-assignment"
    | "worker-resource"
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output"
    | "workstation-resource";
  source: FactoryGraphNodeKey;
  target: FactoryGraphNodeKey;
}

export interface FactoryGraphDraft {
  additions: {
    resources: FactoryResource[];
    workers: FactoryWorker[];
    workStates: Array<{
      state: FactoryWorkState;
      workTypeName: string;
    }>;
    workTypes: FactoryWorkType[];
    workstations: FactoryWorkstation[];
  };
  edgeChanges: {
    additions: FactoryGraphDraftEdgeChange[];
    removals: FactoryGraphDraftEdgeChange[];
  };
  removals: {
    resources: string[];
    workers: string[];
    workStates: Array<{
      stateName: string;
      workTypeName: string;
    }>;
    workTypes: string[];
    workstations: string[];
  };
}

export interface FactoryGraphTopology {
  edges: FactoryGraphEdge[];
  nodes: FactoryGraphNode[];
}

export interface FactoryGraphDraftDerivedState {
  baseDocument: EditableFactoryDefinitionDocument | null;
  draft: FactoryGraphDraft;
  graph: FactoryGraphTopology;
  hasChanges: boolean;
  latestDocument: EditableFactoryDefinitionDocument | null;
  pendingFactoryDefinition: CanonicalFactoryDefinition | null;
  source: "editable-definition" | "projection";
  validationErrors: FactoryGraphDraftValidationError[];
}

export interface FactoryGraphDraftSessionState {
  draft: FactoryGraphDraft;
  latestDocument: EditableFactoryDefinitionDocument;
  sessionStartDocument: EditableFactoryDefinitionDocument;
}

export function createEmptyFactoryGraphDraft(): FactoryGraphDraft {
  return {
    additions: {
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
    edgeChanges: {
      additions: [],
      removals: [],
    },
    removals: {
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
  };
}

export function hasFactoryGraphDraftChanges(draft: FactoryGraphDraft): boolean {
  return Boolean(
    draft.additions.resources.length ||
      draft.additions.workers.length ||
      draft.additions.workStates.length ||
      draft.additions.workTypes.length ||
      draft.additions.workstations.length ||
      draft.edgeChanges.additions.length ||
      draft.edgeChanges.removals.length ||
      draft.removals.resources.length ||
      draft.removals.workers.length ||
      draft.removals.workStates.length ||
      draft.removals.workTypes.length ||
      draft.removals.workstations.length,
  );
}

export function buildNode(key: FactoryGraphNodeKey): FactoryGraphNode {
  return {
    id: nodeKeyId(key),
    key,
    kind: key.kind,
    label:
      key.kind === "work-state"
        ? `${key.workTypeName}:${key.stateName}`
        : key.name,
  };
}

export function buildEdge(
  kind: FactoryGraphEdgeKind,
  source: FactoryGraphNodeKey,
  target: FactoryGraphNodeKey,
): FactoryGraphEdge {
  return {
    id: `${kind}:${nodeKeyId(source)}->${nodeKeyId(target)}`,
    kind,
    source,
    sourceId: nodeKeyId(source),
    target,
    targetId: nodeKeyId(target),
  };
}

export function edgeChangeId(
  edgeChange: FactoryGraphDraftEdgeChange | FactoryGraphEdge,
) {
  return `${edgeChange.kind}:${nodeKeyId(edgeChange.source)}->${nodeKeyId(edgeChange.target)}`;
}

export function nodeKeyId(key: FactoryGraphNodeKey): string {
  switch (key.kind) {
    case "resource":
      return `resource:${key.name}`;
    case "worker":
      return `worker:${key.name}`;
    case "work-state":
      return `work-state:${key.workTypeName}:${key.stateName}`;
    case "work-type":
      return `work-type:${key.name}`;
    case "workstation":
      return `workstation:${key.name}`;
  }
}
