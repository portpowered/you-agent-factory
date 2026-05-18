import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphDraftValidationError,
  FactoryGraphNode,
  FactoryGraphNodeKind,
  FactoryWorkState,
  FactoryWorkstation,
} from "./factory-graph-draft-types";
import { nodeKeyId } from "./factory-graph-draft-types";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";

export function validateFactoryGraphDraft(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
): FactoryGraphDraftValidationError[] {
  const errors: FactoryGraphDraftValidationError[] = [];
  const nodeIndex = indexFactoryGraphNodes(baseFactoryDefinition, draft);

  for (const resource of draft.additions.resources) {
    validateRequiredName(resource.name, "resource", errors);
  }
  for (const worker of draft.additions.workers) {
    validateRequiredName(worker.name, "worker", errors);
  }
  for (const workType of draft.additions.workTypes) {
    validateRequiredName(workType.name, "work-type", errors);
  }
  for (const workstation of draft.additions.workstations) {
    validateRequiredName(workstation.name, "workstation", errors);
    if (workstation.worker.trim().length === 0) {
      errors.push(missingWorkerError(workstation.name));
    }
  }
  for (const workState of draft.additions.workStates) {
    validateRequiredName(workState.state.name, "work-state", errors);
    validateRequiredName(workState.workTypeName, "work-type", errors);
  }

  validateDuplicateIdentifiers(nodeIndex, errors);
  validateEdgeChanges(draft, nodeIndex, errors);
  validateFinalWorkerAssignments(baseFactoryDefinition, draft, errors);

  return dedupeValidationErrors(errors);
}

function validateDuplicateIdentifiers(
  nodeIndex: Map<string, FactoryGraphNode>,
  errors: FactoryGraphDraftValidationError[],
) {
  const seenById = new Set<string>();
  for (const nodeId of nodeIndex.keys()) {
    if (seenById.has(nodeId)) {
      errors.push({
        code: "DUPLICATE_IDENTIFIER",
        message: `Duplicate graph identifier "${nodeId}" is not allowed.`,
        target: {
          kind: "node",
          id: nodeId,
        },
      });
      continue;
    }
    seenById.add(nodeId);
  }
}

function validateEdgeChanges(
  draft: FactoryGraphDraft,
  nodeIndex: Map<string, FactoryGraphNode>,
  errors: FactoryGraphDraftValidationError[],
) {
  for (const edgeChange of draft.edgeChanges.additions) {
    validateEdgeChange(edgeChange, nodeIndex, errors);
  }
  for (const edgeChange of draft.edgeChanges.removals) {
    validateEdgeChange(edgeChange, nodeIndex, errors);
  }
}

function validateFinalWorkerAssignments(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
  errors: FactoryGraphDraftValidationError[],
) {
  const pendingFactoryDefinition = structuredClone(baseFactoryDefinition);
  pendingFactoryDefinition.workstations = applyNamedEntityChanges(
    baseFactoryDefinition.workstations,
    draft.removals.workstations,
    draft.additions.workstations,
  );

  for (const workstation of pendingFactoryDefinition.workstations ?? []) {
    const nextWorkstation = applyWorkstationWorkerAssignments(workstation, draft);
    if (nextWorkstation.worker.trim().length === 0) {
      errors.push(missingWorkerError(nextWorkstation.name));
    }
  }
}

function applyNamedEntityChanges<T extends { name: string }>(
  baseItems: T[] | undefined,
  removals: string[],
  additions: T[],
): T[] {
  const retainedItems = (baseItems ?? []).filter(
    (item) => !removals.includes(item.name),
  );
  return [...retainedItems, ...additions.map((item) => structuredClone(item))];
}

function applyWorkStateChanges(
  baseStates: FactoryWorkState[],
  draft: FactoryGraphDraft,
  workTypeName: string,
): FactoryWorkState[] {
  const removedStateNames = new Set(
    draft.removals.workStates
      .filter((state) => state.workTypeName === workTypeName)
      .map((state) => state.stateName),
  );
  const retainedStates = baseStates.filter(
    (state) => !removedStateNames.has(state.name),
  );
  const addedStates = draft.additions.workStates
    .filter((state) => state.workTypeName === workTypeName)
    .map((state) => structuredClone(state.state));

  return [...retainedStates, ...addedStates];
}

function applyWorkstationWorkerAssignments(
  workstation: FactoryWorkstation,
  draft: FactoryGraphDraft,
): FactoryWorkstation {
  const nextWorkstation = structuredClone(workstation);
  const removedAssignment = draft.edgeChanges.removals.some(
    (edge) =>
      edge.kind === "worker-assignment" &&
      edge.target.kind === "workstation" &&
      edge.target.name === workstation.name &&
      edge.source.kind === "worker" &&
      edge.source.name === nextWorkstation.worker,
  );
  const addedAssignment = draft.edgeChanges.additions.find(
    (edge) =>
      edge.kind === "worker-assignment" &&
      edge.target.kind === "workstation" &&
      edge.target.name === workstation.name &&
      edge.source.kind === "worker",
  );

  if (removedAssignment && !addedAssignment) {
    nextWorkstation.worker = "";
  }
  if (addedAssignment?.source.kind === "worker") {
    nextWorkstation.worker = addedAssignment.source.name;
  }

  return nextWorkstation;
}

function validateRequiredName(
  value: string,
  kind: FactoryGraphNodeKind,
  errors: FactoryGraphDraftValidationError[],
) {
  if (value.trim().length > 0) {
    return;
  }
  errors.push({
    code: "MISSING_REQUIRED_FIELD",
    field: "name",
    message: `${kind} identifiers are required before save.`,
    target: {
      kind: "field",
      field: `${kind}.name`,
    },
  });
}

function validateEdgeChange(
  edgeChange: FactoryGraphDraftEdgeChange,
  nodeIndex: Map<string, FactoryGraphNode>,
  errors: FactoryGraphDraftValidationError[],
) {
  const source = nodeIndex.get(nodeKeyId(edgeChange.source));
  const target = nodeIndex.get(nodeKeyId(edgeChange.target));
  const relationshipId =
    `${edgeChange.kind}:${nodeKeyId(edgeChange.source)}->${nodeKeyId(edgeChange.target)}`;

  if (!source) {
    errors.push(unknownEdgeNodeError(relationshipId, "source"));
    return;
  }
  if (!target) {
    errors.push(unknownEdgeNodeError(relationshipId, "target"));
    return;
  }
  if (!edgeKindsByNodeKinds(source.kind, target.kind).includes(edgeChange.kind)) {
    errors.push({
      code: "INCOMPATIBLE_EDGE",
      message: `Relationship "${edgeChange.kind}" cannot connect ${source.kind} to ${target.kind}.`,
      target: { kind: "edge", id: relationshipId },
    });
  }
}

function edgeKindsByNodeKinds(
  source: FactoryGraphNodeKind,
  target: FactoryGraphNodeKind,
): FactoryGraphDraftEdgeChange["kind"][] {
  if (source === "resource" && target === "worker") {
    return ["worker-resource"];
  }
  if (source === "resource" && target === "workstation") {
    return ["workstation-resource"];
  }
  if (source === "worker" && target === "workstation") {
    return ["worker-assignment"];
  }
  if (source === "work-state" && target === "workstation") {
    return ["workstation-input"];
  }
  if (source === "workstation" && target === "work-state") {
    return [
      "workstation-on-continue",
      "workstation-on-failure",
      "workstation-on-rejection",
      "workstation-output",
    ];
  }
  return [];
}

function dedupeValidationErrors(
  errors: FactoryGraphDraftValidationError[],
): FactoryGraphDraftValidationError[] {
  const seen = new Set<string>();
  return errors.filter((error) => {
    const key = `${error.code}:${error.message}:${JSON.stringify(error.target)}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function indexFactoryGraphNodes(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
): Map<string, FactoryGraphNode> {
  const pendingFactoryDefinition = structuredClone(baseFactoryDefinition);
  pendingFactoryDefinition.resources = applyNamedEntityChanges(
    baseFactoryDefinition.resources,
    draft.removals.resources,
    draft.additions.resources,
  );
  pendingFactoryDefinition.workers = applyNamedEntityChanges(
    baseFactoryDefinition.workers,
    draft.removals.workers,
    draft.additions.workers,
  );
  pendingFactoryDefinition.workstations = applyNamedEntityChanges(
    baseFactoryDefinition.workstations,
    draft.removals.workstations,
    draft.additions.workstations,
  );
  pendingFactoryDefinition.workTypes = applyNamedEntityChanges(
    baseFactoryDefinition.workTypes,
    draft.removals.workTypes,
    draft.additions.workTypes,
  ).map((workType) => ({
    ...workType,
    states: applyWorkStateChanges(workType.states, draft, workType.name),
  }));

  return new Map(
    buildFactoryGraphTopologyFromDefinition(pendingFactoryDefinition).nodes.map(
      (node) => [node.id, node],
    ),
  );
}

function missingWorkerError(
  workstationName: string,
): FactoryGraphDraftValidationError {
  return {
    code: "MISSING_REQUIRED_FIELD",
    field: "worker",
    message: `Workstation "${workstationName}" must keep one worker assignment.`,
    target: {
      kind: "node",
      id: nodeKeyId({
        kind: "workstation",
        name: workstationName,
      }),
    },
  };
}

function unknownEdgeNodeError(
  relationshipId: string,
  which: "source" | "target",
): FactoryGraphDraftValidationError {
  return {
    code: "UNKNOWN_NODE",
    message: `Relationship "${relationshipId}" references an unknown ${which} node.`,
    target: { kind: "edge", id: relationshipId },
  };
}
