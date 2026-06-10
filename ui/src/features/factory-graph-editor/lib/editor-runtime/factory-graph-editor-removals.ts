import { getFactoryGraphEditorMessages } from "../../messages/editor";
import { buildPendingFactoryDefinition } from "../draft/factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphEdge,
  FactoryGraphNodeKey,
} from "../draft/factory-graph-draft-types";
import { edgeChangeId, nodeKeyId } from "../draft/factory-graph-draft-types";
import {
  buildEdgeRemovalDescription,
  describeEdgeLabel,
} from "../editor/factory-graph-editor-edge-removal-copy";

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
  requiresConfirmation: boolean;
  title: string;
}

export interface FactoryGraphEdgeRemovalIntent {
  confirmDescription: string;
  confirmLabel: string;
  edge: FactoryGraphEdge;
  ineligibleReason?: string;
  requiresConfirmation: boolean;
  title: string;
}

export function buildFactoryGraphRemovalIntent(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  locale?: string | null;
  nodeId: string;
}): FactoryGraphRemovalIntent | null {
  const messages = getFactoryGraphEditorMessages(options.locale);
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(
      options.baseFactoryDefinition,
      options.draft,
    ) ?? options.baseFactoryDefinition;
  const currentTopology = buildFactoryGraphTopologyFromDefinition(
    currentFactoryDefinition,
  );
  const node = currentTopology.nodes.find(
    (entry) => entry.id === options.nodeId,
  );

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
  const impactedStateCount = removedWorkTypeName
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
      ineligibleReason: messages.removalWorkerAssignedReason(
        workstationAssignments.length,
        node.label,
      ),
      key: node.key,
      requiresConfirmation: false,
      title: "",
    };
  }

  const requiresConfirmation =
    connectedEdges.length > 0 || impactedStateCount > 0;

  return {
    confirmDescription: buildRemovalDescription(
      node.kind,
      node.label,
      connectedEdges.length,
      impactedStateCount,
      options.locale,
    ),
    confirmLabel: messages.removalEntityConfirmLabel(node.label, node.kind),
    key: node.key,
    requiresConfirmation,
    title: messages.removalEntityTitle(node.label, node.kind),
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
  locale?: string | null;
}): FactoryGraphEdgeRemovalIntent | null {
  const messages = getFactoryGraphEditorMessages(options.locale);
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(
      options.baseFactoryDefinition,
      options.draft,
    ) ?? options.baseFactoryDefinition;
  const currentTopology = buildFactoryGraphTopologyFromDefinition(
    currentFactoryDefinition,
  );
  const edge = currentTopology.edges.find(
    (entry) => entry.id === options.edgeId,
  );

  if (!edge) {
    return null;
  }
  if (edge.kind === "work-type-state") {
    return {
      confirmDescription: "",
      confirmLabel: "",
      edge,
      ineligibleReason: messages.removalEdgeIneligibleWorkTypeState,
      requiresConfirmation: false,
      title: "",
    };
  }

  const edgeLabel = describeEdgeLabel(edge, options.locale);
  return {
    confirmDescription: buildEdgeRemovalDescription(edge, options.locale),
    confirmLabel: messages.removalEdgeConfirmLabel(edgeLabel),
    edge,
    requiresConfirmation: true,
    title: messages.removalEdgeTitle(edgeLabel),
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
  for (const targetPath of draft.removals.docs) {
    pendingNodeIds.add(`doc:${targetPath}`);
  }

  return pendingNodeIds;
}

function removeDraftAddition(
  draft: FactoryGraphDraft,
  key: FactoryGraphNodeKey,
) {
  switch (key.kind) {
    case "doc":
      draft.additions.docs = draft.additions.docs.filter(
        (doc) => doc.targetPath !== key.name,
      );
      return;
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
        draft.removals.resources = appendUnique(
          draft.removals.resources,
          key.name,
        );
      }
      return;
    case "worker":
      if (entityExists(baseFactoryDefinition.workers, key.name)) {
        draft.removals.workers = appendUnique(draft.removals.workers, key.name);
      }
      return;
    case "work-type":
      if (entityExists(baseFactoryDefinition.workTypes, key.name)) {
        draft.removals.workTypes = appendUnique(
          draft.removals.workTypes,
          key.name,
        );
        const ownedStates =
          baseFactoryDefinition.workTypes?.find(
            (workType) => workType.name === key.name,
          )?.states ?? [];
        for (const state of ownedStates) {
          draft.removals.workStates = appendUniqueBy(
            draft.removals.workStates,
            {
              stateName: state.name,
              workTypeName: key.name,
            },
            (entry) => `${entry.workTypeName}:${entry.stateName}`,
          );
        }
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

function pruneEdgeChangesForNode(
  draft: FactoryGraphDraft,
  key: FactoryGraphNodeKey,
) {
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
  const currentTopology = buildFactoryGraphTopologyFromDefinition(
    currentFactoryDefinition,
  );

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

function shouldRemoveEdgeForNode(
  edge: FactoryGraphEdge,
  key: FactoryGraphNodeKey,
) {
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
      (edge.source.kind === "work-state" &&
        edge.source.workTypeName === key.name &&
        edge.kind !== "work-type-state") ||
      (edge.target.kind === "work-state" &&
        edge.target.workTypeName === key.name &&
        edge.kind !== "work-type-state"))
  );
}

function toDraftEdgeRemoval(
  edge: FactoryGraphEdge,
): FactoryGraphDraftEdgeChange | null {
  if (
    edge.kind === "work-type-state" ||
    edge.kind === "work-state-visibility-bypass"
  ) {
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

function entityExists<T extends { name: string }>(
  items: T[] | undefined,
  name: string,
) {
  return items?.some((item) => item.name === name) ?? false;
}

function appendUnique<T>(items: T[], value: T): T[] {
  return items.includes(value) ? items : [...items, value];
}

function appendUniqueBy<T>(
  items: T[],
  value: T,
  getKey: (value: T) => string,
): T[] {
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
  locale?: string | null,
) {
  const messages = getFactoryGraphEditorMessages(locale);
  return messages.removalDescription({
    connectedEdgeCount,
    impactedStateCount,
    kind,
    label,
  });
}
