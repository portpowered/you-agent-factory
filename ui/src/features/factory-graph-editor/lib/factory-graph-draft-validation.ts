import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphDraftValidationError,
  FactoryGraphNode,
  FactoryGraphNodeKind,
} from "./factory-graph-draft-types";
import { buildNode, nodeKeyId } from "./factory-graph-draft-types";
import { buildDraftAppliedFactoryDefinition } from "./factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";

export function validateFactoryGraphDraft(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
): FactoryGraphDraftValidationError[] {
  const errors: FactoryGraphDraftValidationError[] = [];
  const pendingFactoryDefinition = buildDraftAppliedFactoryDefinition(
    baseFactoryDefinition,
    draft,
  );
  const nodeIndex = new Map(
    buildFactoryGraphTopologyFromDefinition(pendingFactoryDefinition).nodes.map(
      (node) => [node.id, node],
    ),
  );
  seedRemovalEdgeNodes(draft, nodeIndex);

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

  validateDuplicateIdentifiers(pendingFactoryDefinition, errors);
  validateEdgeChanges(draft, nodeIndex, errors);
  validateFinalWorkerAssignments(pendingFactoryDefinition, errors);

  return dedupeValidationErrors(errors);
}

function validateDuplicateIdentifiers(
  factoryDefinition: CanonicalFactoryDefinition,
  errors: FactoryGraphDraftValidationError[],
) {
  const countsById = new Map<string, number>();

  for (const nodeId of collectFactoryGraphNodeIds(factoryDefinition)) {
    countsById.set(nodeId, (countsById.get(nodeId) ?? 0) + 1);
  }

  for (const [nodeId, count] of countsById) {
    if (count > 1) {
      errors.push({
        code: "DUPLICATE_IDENTIFIER",
        message: `Duplicate graph identifier "${nodeId}" is not allowed.`,
        target: {
          kind: "node",
          id: nodeId,
        },
      });
    }
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

function seedRemovalEdgeNodes(
  draft: FactoryGraphDraft,
  nodeIndex: Map<string, FactoryGraphNode>,
) {
  for (const edgeChange of draft.edgeChanges.removals) {
    const sourceId = nodeKeyId(edgeChange.source);
    if (!nodeIndex.has(sourceId)) {
      nodeIndex.set(sourceId, buildNode(edgeChange.source));
    }

    const targetId = nodeKeyId(edgeChange.target);
    if (!nodeIndex.has(targetId)) {
      nodeIndex.set(targetId, buildNode(edgeChange.target));
    }
  }
}

function validateFinalWorkerAssignments(
  pendingFactoryDefinition: CanonicalFactoryDefinition,
  errors: FactoryGraphDraftValidationError[],
) {
  for (const workstation of pendingFactoryDefinition.workstations ?? []) {
    if (workstation.worker.trim().length === 0) {
      errors.push(missingWorkerError(workstation.name));
    }
  }
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

function collectFactoryGraphNodeIds(
  factoryDefinition: CanonicalFactoryDefinition,
): string[] {
  const nodeIds: string[] = [];

  for (const resource of factoryDefinition.resources ?? []) {
    nodeIds.push(
      nodeKeyId({
        kind: "resource",
        name: resource.name,
      }),
    );
  }
  for (const worker of factoryDefinition.workers ?? []) {
    nodeIds.push(
      nodeKeyId({
        kind: "worker",
        name: worker.name,
      }),
    );
  }
  for (const workType of factoryDefinition.workTypes ?? []) {
    nodeIds.push(
      nodeKeyId({
        kind: "work-type",
        name: workType.name,
      }),
    );
    for (const state of workType.states) {
      nodeIds.push(
        nodeKeyId({
          kind: "work-state",
          stateName: state.name,
          workTypeName: workType.name,
        }),
      );
    }
  }
  for (const workstation of factoryDefinition.workstations ?? []) {
    nodeIds.push(
      nodeKeyId({
        kind: "workstation",
        name: workstation.name,
      }),
    );
  }

  return nodeIds;
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
