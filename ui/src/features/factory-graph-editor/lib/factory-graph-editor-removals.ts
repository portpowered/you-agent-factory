import { buildPendingFactoryDefinition } from "./factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import {
  buildEdgeRemovalDescription,
  describeEdgeLabel,
} from "./factory-graph-editor-edge-removal-copy";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphEdge,
  FactoryGraphNodeKey,
} from "./factory-graph-draft-types";
import { edgeChangeId, nodeKeyId } from "./factory-graph-draft-types";

const REMOVABLE_NODE_KINDS = new Set([
  "resource",
  "worker",
  "work-state",
  "work-type",
  "workstation",
]);

export interface FactoryGraphRemovalIntent {
  confirmDescription: string;
  confirmLabel: string;
  ineligibleReason?: string;
  key: FactoryGraphNodeKey;
  title: string;
}

export interface FactoryGraphEdgeRemovalIntent {
  confirmDescription: string;
  confirmLabel: string;
  edge: FactoryGraphEdge;
  ineligibleReason?: string;
  title: string;
}

export function buildFactoryGraphRemovalIntent(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  nodeId: string;
}): FactoryGraphRemovalIntent | null {
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(options.baseFactoryDefinition, options.draft) ??
    options.baseFactoryDefinition;
  const currentTopology =
    buildFactoryGraphTopologyFromDefinition(currentFactoryDefinition);
  const node = currentTopology.nodes.find((entry) => entry.id === options.nodeId);

  if (!node || !REMOVABLE_NODE_KINDS.has(node.kind)) {
    return null;
  }

  const connectedEdges = currentTopology.edges.filter(
    (edge) => edge.sourceId === node.id || edge.targetId === node.id,
  );
  const workstationAssignments = connectedEdges.filter(
    (edge) => edge.kind === "worker-assignment",
  );
  const removedWorkTypeName =
    node.kind === "work-type" && node.key.kind === "work-type"
      ? node.key.name
      : null;
  const impactedStateCount =
    removedWorkTypeName
      ? currentTopology.nodes.filter(
          (entry) =>
            entry.kind === "work-state" &&
            entry.key.kind === "work-state" &&
            entry.key.workTypeName === removedWorkTypeName,
        ).length
      : 0;

  if (node.kind === "worker" && workstationAssignments.length > 0) {
    return {
      confirmDescription: "",
      confirmLabel: "",
      ineligibleReason: `This worker is still assigned to ${pluralize(
        workstationAssignments.length,
        "workstation",
      )}. Reassign or remove those workstations before deleting ${node.label}.`,
      key: node.key,
      title: "",
    };
  }

  return {
    confirmDescription: buildRemovalDescription(
      node.kind,
      node.label,
      connectedEdges.length,
      impactedStateCount,
    ),
    confirmLabel: `Delete ${node.label} ${node.kind}`,
    key: node.key,
    title: `Remove ${node.label} ${node.kind}?`,
  };
}

export function applyFactoryGraphEntityRemoval(
  currentDraft: FactoryGraphDraft,
  baseFactoryDefinition: CanonicalFactoryDefinition,
  key: FactoryGraphNodeKey,
): FactoryGraphDraft {
  const nextDraft = structuredClone(currentDraft);
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(baseFactoryDefinition, currentDraft) ??
    baseFactoryDefinition;

  removeDraftAddition(nextDraft, key);
  addDraftRemoval(nextDraft, key, baseFactoryDefinition);
  pruneEdgeChangesForNode(nextDraft, key);
  addRelationshipRemovals(nextDraft, currentFactoryDefinition, key);

  return nextDraft;
}

export function buildFactoryGraphEdgeRemovalIntent(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  edgeId: string;
}): FactoryGraphEdgeRemovalIntent | null {
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(options.baseFactoryDefinition, options.draft) ??
    options.baseFactoryDefinition;
  const currentTopology =
    buildFactoryGraphTopologyFromDefinition(currentFactoryDefinition);
  const edge = currentTopology.edges.find((entry) => entry.id === options.edgeId);

  if (!edge) {
    return null;
  }
  if (edge.kind === "work-type-state") {
    return {
      confirmDescription: "",
      confirmLabel: "",
      edge,
      ineligibleReason:
        "Work type ordering edges are managed by work-state membership and cannot be removed directly.",
      title: "",
    };
  }

  return {
    confirmDescription: buildEdgeRemovalDescription(edge),
    confirmLabel: `Remove ${describeEdgeLabel(edge)}`,
    edge,
    title: `Remove ${describeEdgeLabel(edge)}?`,
  };
}

export function collectPendingRemovalNodeIds(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
): Set<string> {
  const pendingNodeIds = new Set<string>();

  for (const name of draft.removals.resources) {
    pendingNodeIds.add(nodeKeyId({ kind: "resource", name }));
  }
  for (const name of draft.removals.workers) {
    pendingNodeIds.add(nodeKeyId({ kind: "worker", name }));
  }
  for (const workTypeName of draft.removals.workTypes) {
    pendingNodeIds.add(nodeKeyId({ kind: "work-type", name: workTypeName }));
    const workType = baseFactoryDefinition.workTypes?.find(
      (entry) => entry.name === workTypeName,
    );
    for (const state of workType?.states ?? []) {
      pendingNodeIds.add(
        nodeKeyId({
          kind: "work-state",
          stateName: state.name,
          workTypeName,
        }),
      );
    }
  }
  for (const state of draft.removals.workStates) {
    pendingNodeIds.add(
      nodeKeyId({
        kind: "work-state",
        stateName: state.stateName,
        workTypeName: state.workTypeName,
      }),
    );
  }
  for (const name of draft.removals.workstations) {
    pendingNodeIds.add(nodeKeyId({ kind: "workstation", name }));
  }

  return pendingNodeIds;
}

export function collectPendingRemovalEdgeIds(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
): Set<string> {
  const pendingNodeIds = collectPendingRemovalNodeIds(baseFactoryDefinition, draft);
  const pendingEdgeIds = new Set<string>();
  const baseTopology = buildFactoryGraphTopologyFromDefinition(baseFactoryDefinition);

  for (const edge of baseTopology.edges) {
    if (
      pendingNodeIds.has(edge.sourceId) ||
      pendingNodeIds.has(edge.targetId) ||
      matchesExplicitEdgeRemoval(edge, draft.edgeChanges.removals)
    ) {
      pendingEdgeIds.add(edge.id);
    }
  }

  return pendingEdgeIds;
}

function removeDraftAddition(draft: FactoryGraphDraft, key: FactoryGraphNodeKey) {
  switch (key.kind) {
    case "resource":
      draft.additions.resources = draft.additions.resources.filter(
        (resource) => resource.name !== key.name,
      );
      return;
    case "worker":
      draft.additions.workers = draft.additions.workers.filter(
        (worker) => worker.name !== key.name,
      );
      return;
    case "work-type":
      draft.additions.workTypes = draft.additions.workTypes.filter(
        (workType) => workType.name !== key.name,
      );
      draft.additions.workStates = draft.additions.workStates.filter(
        (state) => state.workTypeName !== key.name,
      );
      return;
    case "work-state":
      draft.additions.workStates = draft.additions.workStates.filter(
        (state) =>
          !(
            state.workTypeName === key.workTypeName &&
            state.state.name === key.stateName
          ),
      );
      return;
    case "workstation":
      draft.additions.workstations = draft.additions.workstations.filter(
        (workstation) => workstation.name !== key.name,
      );
  }
}

function addDraftRemoval(
  draft: FactoryGraphDraft,
  key: FactoryGraphNodeKey,
  baseFactoryDefinition: CanonicalFactoryDefinition,
) {
  switch (key.kind) {
    case "resource":
      if (entityExists(baseFactoryDefinition.resources, key.name)) {
        draft.removals.resources = appendUnique(draft.removals.resources, key.name);
      }
      return;
    case "worker":
      if (entityExists(baseFactoryDefinition.workers, key.name)) {
        draft.removals.workers = appendUnique(draft.removals.workers, key.name);
      }
      return;
    case "work-type":
      if (entityExists(baseFactoryDefinition.workTypes, key.name)) {
        draft.removals.workTypes = appendUnique(draft.removals.workTypes, key.name);
      }
      return;
    case "work-state":
      if (
        baseFactoryDefinition.workTypes?.some(
          (workType) =>
            workType.name === key.workTypeName &&
            workType.states.some((state) => state.name === key.stateName),
        )
      ) {
        draft.removals.workStates = appendUniqueBy(
          draft.removals.workStates,
          {
            stateName: key.stateName,
            workTypeName: key.workTypeName,
          },
          (state) => `${state.workTypeName}:${state.stateName}`,
        );
      }
      return;
    case "workstation":
      if (entityExists(baseFactoryDefinition.workstations, key.name)) {
        draft.removals.workstations = appendUnique(
          draft.removals.workstations,
          key.name,
        );
      }
  }
}

function pruneEdgeChangesForNode(draft: FactoryGraphDraft, key: FactoryGraphNodeKey) {
  draft.edgeChanges.additions = draft.edgeChanges.additions.filter(
    (edge) => !edgeTouchesNode(edge, key),
  );
  draft.edgeChanges.removals = draft.edgeChanges.removals.filter(
    (edge) => !edgeTouchesNode(edge, key),
  );
}

function addRelationshipRemovals(
  draft: FactoryGraphDraft,
  currentFactoryDefinition: CanonicalFactoryDefinition,
  key: FactoryGraphNodeKey,
) {
  const currentTopology =
    buildFactoryGraphTopologyFromDefinition(currentFactoryDefinition);

  for (const edge of currentTopology.edges) {
    if (!shouldRemoveEdgeForNode(edge, key)) {
      continue;
    }

    const removal = toDraftEdgeRemoval(edge);
    if (!removal) {
      continue;
    }

    draft.edgeChanges.removals = appendUniqueBy(
      draft.edgeChanges.removals,
      removal,
      edgeChangeId,
    );
  }
}

function shouldRemoveEdgeForNode(edge: FactoryGraphEdge, key: FactoryGraphNodeKey) {
  if (key.kind === "workstation") {
    return false;
  }

  if (
    key.kind === "resource" ||
    key.kind === "worker" ||
    key.kind === "work-state"
  ) {
    return edgeTouchesNode(edge, key) && edge.kind !== "work-type-state";
  }

  return (
    key.kind === "work-type" &&
    (edge.sourceId === nodeKeyId(key) ||
      (edge.target.kind === "work-state" &&
        edge.target.workTypeName === key.name &&
        edge.kind !== "work-type-state"))
  );
}

function matchesExplicitEdgeRemoval(
  edge: FactoryGraphEdge,
  removals: FactoryGraphDraftEdgeChange[],
) {
  return removals.some((removal) => edgeChangeId(removal) === edge.id);
}

function toDraftEdgeRemoval(
  edge: FactoryGraphEdge,
): FactoryGraphDraftEdgeChange | null {
  if (edge.kind === "work-type-state") {
    return null;
  }

  return {
    kind: edge.kind,
    source: edge.source,
    target: edge.target,
  };
}

function edgeTouchesNode(
  edge: Pick<FactoryGraphEdge, "source" | "target">,
  key: FactoryGraphNodeKey,
) {
  return nodeKeyMatches(edge.source, key) || nodeKeyMatches(edge.target, key);
}

function nodeKeyMatches(left: FactoryGraphNodeKey, right: FactoryGraphNodeKey) {
  return nodeKeyId(left) === nodeKeyId(right);
}

function entityExists<T extends { name: string }>(items: T[] | undefined, name: string) {
  return items?.some((item) => item.name === name) ?? false;
}

function appendUnique<T>(items: T[], value: T): T[] {
  return items.includes(value) ? items : [...items, value];
}

function appendUniqueBy<T>(items: T[], value: T, getKey: (value: T) => string): T[] {
  const valueKey = getKey(value);
  return items.some((item) => getKey(item) === valueKey)
    ? items
    : [...items, value];
}

function buildRemovalDescription(
  kind: FactoryGraphNodeKey["kind"],
  label: string,
  connectedEdgeCount: number,
  impactedStateCount: number,
) {
  const edgeSummary =
    connectedEdgeCount > 0
      ? `This will remove ${pluralize(connectedEdgeCount, "graph edge")}.`
      : "This entity has no connected graph edges to remove.";

  if (kind === "work-type") {
    return `${edgeSummary} ${label} also owns ${pluralize(
      impactedStateCount,
      "work state",
    )}, which will be removed with it.`;
  }

  if (kind === "work-state") {
    return `${edgeSummary} Any workstation routes that reference ${label} will be cleared from the pending draft.`;
  }

  if (kind === "resource") {
    return `${edgeSummary} Worker and workstation resource references that depend on ${label} will be cleared from the pending draft.`;
  }

  return edgeSummary;
}

function pluralize(count: number, noun: string) {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}
