import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
} from "../../../../api/current-factory-definition";

type FactoryResource = NonNullable<
  CanonicalFactoryDefinition["resources"]
>[number];
type FactoryWorker = NonNullable<CanonicalFactoryDefinition["workers"]>[number];
type FactoryWorkType = NonNullable<
  CanonicalFactoryDefinition["workTypes"]
>[number];
type FactoryWorkState = FactoryWorkType["states"][number];
type FactoryWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type FactoryWorkstationIO = FactoryWorkstation["inputs"][number];

export type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
  FactoryResource,
  FactoryWorker,
  FactoryWorkState,
  FactoryWorkstation,
  FactoryWorkstationIO,
  FactoryWorkType,
};

export type FactoryGraphNodeFieldUpdate =
  | {
      field: "capacity";
      kind: "resource";
      name: string;
      value: FactoryResource["capacity"];
    }
  | {
      field: "model";
      kind: "worker";
      name: string;
      value: FactoryWorker["model"];
    }
  | {
      field: "type";
      kind: "work-state";
      stateName: string;
      value: FactoryWorkState["type"];
      workTypeName: string;
    }
  | {
      field: "body" | "worker";
      kind: "workstation";
      name: string;
      value: string;
    }
  | {
      field: "behavior";
      kind: "workstation";
      name: string;
      value: FactoryWorkstation["behavior"];
    };

export type FactoryGraphNodeKind =
  | "doc"
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
  | "work-state-visibility-bypass"
  | "work-type-state";

export interface FactoryGraphNodeReference {
  id?: string;
  kind: Exclude<FactoryGraphNodeKind, "work-state">;
  name: string;
  sourceFileType?: string;
}

export interface FactoryGraphWorkStateReference {
  kind: "work-state";
  stateId?: string;
  stateName: string;
  workTypeId?: string;
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
  /** Outcome route label for display-only work-state bypass edges. */
  outcomeRouteKind?: WorkstationToWorkStateRouteKind;
  source: FactoryGraphNodeKey;
  sourceId: string;
  target: FactoryGraphNodeKey;
  targetId: string;
}

export type WorkstationToWorkStateRouteKind =
  | "workstation-on-continue"
  | "workstation-on-failure"
  | "workstation-on-rejection"
  | "workstation-output";

export interface FactoryGraphDraftValidationError {
  code:
    | "DUPLICATE_IDENTIFIER"
    | "INCOMPATIBLE_EDGE"
    | "INVALID_RELATIONSHIP_TYPE"
    | "INVALID_WORKSTATION_ROUTE"
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

export interface FactoryGraphDocDraft {
  inlineContent: string;
  targetPath: string;
}

export interface FactoryGraphDraft {
  additions: {
    docs: FactoryGraphDocDraft[];
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
  /** Canonical field updates, kept separate from disposable graph projections. */
  fieldChanges?: FactoryGraphNodeFieldUpdate[];
  removals: {
    docs: string[];
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
  baseDocument: CurrentFactoryDocument | null;
  draft: FactoryGraphDraft;
  graph: FactoryGraphTopology;
  hasChanges: boolean;
  latestDocument: CurrentFactoryDocument | null;
  pendingFactoryDefinition: CanonicalFactoryDefinition | null;
  adoptSavedFactoryDocument: (document: CurrentFactoryDocument) => void;
  replaceDraft: (draft: FactoryGraphDraft) => void;
  resetDraft: () => void;
  source: "current-factory" | "projection";
  updateDraft: (
    updater: (draft: FactoryGraphDraft) => FactoryGraphDraft,
  ) => void;
  validationErrors: FactoryGraphDraftValidationError[];
}

export interface FactoryGraphDraftSessionState {
  draft: FactoryGraphDraft;
  latestDocument: CurrentFactoryDocument;
  sessionStartDocument: CurrentFactoryDocument;
}

export function createEmptyFactoryGraphDraft(): FactoryGraphDraft {
  return {
    additions: {
      docs: [],
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
    fieldChanges: [],
    removals: {
      docs: [],
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
    draft.additions.docs.length ||
      draft.additions.resources.length ||
      draft.additions.workers.length ||
      draft.additions.workStates.length ||
      draft.additions.workTypes.length ||
      draft.additions.workstations.length ||
      draft.edgeChanges.additions.length ||
      draft.edgeChanges.removals.length ||
      draft.fieldChanges?.length ||
      draft.removals.docs.length ||
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

export function appendUniqueEdgeChange(
  edges: FactoryGraphDraftEdgeChange[],
  edgeChange: FactoryGraphDraftEdgeChange,
) {
  const nextEdgeId = edgeChangeId(edgeChange);
  if (edges.some((entry) => edgeChangeId(entry) === nextEdgeId)) {
    return edges;
  }
  return [...edges, edgeChange];
}

export function nodeKeyId(key: FactoryGraphNodeKey): string {
  switch (key.kind) {
    case "doc":
      return factoryGraphNodeIdForSubject("doc", key.id, key.name);
    case "resource":
      return factoryGraphNodeIdForSubject("resource", key.id, key.name);
    case "worker":
      return factoryGraphNodeIdForSubject("worker", key.id, key.name);
    case "work-state":
      return [
        "work-state",
        factoryGraphEntityIdentifier(key.workTypeId, key.workTypeName),
        factoryGraphEntityIdentifier(key.stateId, key.stateName),
      ].join(":");
    case "work-type":
      return factoryGraphNodeIdForSubject("work-type", key.id, key.name);
    case "workstation":
      return factoryGraphNodeIdForSubject("workstation", key.id, key.name);
  }
}

function factoryGraphEntityIdentifier(
  explicitId: string | undefined,
  fallbackName: string,
): string {
  const normalizedId = explicitId?.trim();
  return normalizedId && normalizedId.length > 0 ? normalizedId : fallbackName;
}

function factoryGraphNodeIdForSubject(
  kind: Exclude<FactoryGraphNodeKind, "work-state">,
  explicitId: string | undefined,
  fallbackName: string,
): string {
  return `${kind}:${factoryGraphEntityIdentifier(explicitId, fallbackName)}`;
}

function parseFactoryGraphNodeSubjectId(
  nodeId: string,
  prefix: string,
): string | null {
  if (!nodeId.startsWith(prefix)) {
    return null;
  }

  const subjectId = nodeId.slice(prefix.length);
  return subjectId.length > 0 ? subjectId : null;
}

export function parseFactoryGraphWorkstationNodeId(
  nodeId: string,
): string | null {
  return parseFactoryGraphNodeSubjectId(nodeId, "workstation:");
}

export function parseFactoryGraphWorkTypeNodeId(nodeId: string): string | null {
  return parseFactoryGraphNodeSubjectId(nodeId, "work-type:");
}

export function parseFactoryGraphWorkStateNodeId(
  nodeId: string,
): string | null {
  const subjectId = parseFactoryGraphNodeSubjectId(nodeId, "work-state:");
  if (!subjectId) {
    return null;
  }

  const separatorIndex = subjectId.indexOf(":");
  if (separatorIndex <= 0 || separatorIndex >= subjectId.length - 1) {
    return null;
  }

  return subjectId;
}
