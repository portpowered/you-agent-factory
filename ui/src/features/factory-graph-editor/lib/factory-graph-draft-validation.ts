import { workstationRequiresWorkerAssignment } from "../../current-factory-definition/lib/workstation-worker-assignment";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import { buildDraftAppliedFactoryDefinition } from "./factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphDraftValidationError,
  FactoryGraphNode,
  FactoryGraphNodeKind,
} from "./factory-graph-draft-types";
import { buildNode, nodeKeyId } from "./factory-graph-draft-types";

export function validateFactoryGraphDraft(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
  locale?: string | null,
): FactoryGraphDraftValidationError[] {
  return dedupeValidationErrors([
    ...validateFactoryGraphDraftStructural(
      baseFactoryDefinition,
      draft,
      locale,
    ),
    ...validateFactoryGraphDraftSave(baseFactoryDefinition, draft, locale),
  ]);
}

export function validateFactoryGraphDraftStructural(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
  locale?: string | null,
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
    validateRequiredName(resource.name, "resource", errors, locale);
  }
  for (const worker of draft.additions.workers) {
    validateRequiredName(worker.name, "worker", errors, locale);
  }
  for (const workType of draft.additions.workTypes) {
    validateRequiredName(workType.name, "work-type", errors, locale);
  }
  for (const workstation of draft.additions.workstations) {
    validateRequiredName(workstation.name, "workstation", errors, locale);
  }
  for (const workState of draft.additions.workStates) {
    validateRequiredName(workState.state.name, "work-state", errors, locale);
    validateRequiredName(workState.workTypeName, "work-type", errors, locale);
  }

  validateDuplicateIdentifiers(pendingFactoryDefinition, errors, locale);
  validateEdgeChanges(draft, nodeIndex, errors, locale);

  return errors;
}

export function validateFactoryGraphDraftSave(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
  locale?: string | null,
): FactoryGraphDraftValidationError[] {
  const errors: FactoryGraphDraftValidationError[] = [];
  const pendingFactoryDefinition = buildDraftAppliedFactoryDefinition(
    baseFactoryDefinition,
    draft,
  );

  for (const workstation of draft.additions.workstations) {
    if (
      workstationRequiresWorkerAssignment(workstation) &&
      workstation.worker.trim().length === 0
    ) {
      errors.push(missingWorkerError(workstation.name, locale));
    }
  }

  validateFinalWorkerAssignments(pendingFactoryDefinition, errors, locale);

  return errors;
}

function validateDuplicateIdentifiers(
  factoryDefinition: CanonicalFactoryDefinition,
  errors: FactoryGraphDraftValidationError[],
  locale?: string | null,
) {
  const messages = getFactoryGraphEditorMessages(locale);
  const countsById = new Map<string, number>();

  for (const nodeId of collectFactoryGraphNodeIds(factoryDefinition)) {
    countsById.set(nodeId, (countsById.get(nodeId) ?? 0) + 1);
  }

  for (const [nodeId, count] of countsById) {
    if (count > 1) {
      errors.push({
        code: "DUPLICATE_IDENTIFIER",
        message: messages.validationDuplicateIdentifier(nodeId),
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
  locale?: string | null,
) {
  for (const edgeChange of draft.edgeChanges.additions) {
    validateEdgeChange(edgeChange, nodeIndex, errors, locale);
  }
  for (const edgeChange of draft.edgeChanges.removals) {
    validateEdgeChange(edgeChange, nodeIndex, errors, locale);
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
  locale?: string | null,
) {
  for (const workstation of pendingFactoryDefinition.workstations ?? []) {
    if (
      workstationRequiresWorkerAssignment(workstation) &&
      workstation.worker.trim().length === 0
    ) {
      errors.push(missingWorkerError(workstation.name, locale));
    }
  }
}

function validateRequiredName(
  value: string,
  kind: FactoryGraphNodeKind,
  errors: FactoryGraphDraftValidationError[],
  locale?: string | null,
) {
  if (value.trim().length > 0) {
    return;
  }
  const messages = getFactoryGraphEditorMessages(locale);
  errors.push({
    code: "MISSING_REQUIRED_FIELD",
    field: "name",
    message: messages.validationMissingRequiredIdentifier(kind),
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
  locale?: string | null,
) {
  const messages = getFactoryGraphEditorMessages(locale);
  const source = nodeIndex.get(nodeKeyId(edgeChange.source));
  const target = nodeIndex.get(nodeKeyId(edgeChange.target));
  const relationshipId = `${edgeChange.kind}:${nodeKeyId(edgeChange.source)}->${nodeKeyId(edgeChange.target)}`;

  if (!source) {
    errors.push(unknownEdgeNodeError(relationshipId, "source", locale));
    return;
  }
  if (!target) {
    errors.push(unknownEdgeNodeError(relationshipId, "target", locale));
    return;
  }
  if (
    !edgeKindsByNodeKinds(source.kind, target.kind).includes(edgeChange.kind)
  ) {
    errors.push({
      code: "INCOMPATIBLE_EDGE",
      message: messages.validationIncompatibleEdge(
        edgeChange.kind,
        source.kind,
        target.kind,
      ),
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
  locale?: string | null,
): FactoryGraphDraftValidationError {
  const messages = getFactoryGraphEditorMessages(locale);
  return {
    code: "MISSING_REQUIRED_FIELD",
    field: "worker",
    message: messages.validationMissingWorkerAssignment(workstationName),
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
  locale?: string | null,
): FactoryGraphDraftValidationError {
  const messages = getFactoryGraphEditorMessages(locale);
  return {
    code: "UNKNOWN_NODE",
    message: messages.validationUnknownEdgeNode(relationshipId, which),
    target: { kind: "edge", id: relationshipId },
  };
}
