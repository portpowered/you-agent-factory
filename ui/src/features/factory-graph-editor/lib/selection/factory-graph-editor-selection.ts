import type { EdgeChange, NodeChange } from "@xyflow/react";

export type FactoryGraphEditorSelectionTarget =
  | { kind: "node"; id: string }
  | { kind: "edge"; id: string };

export type FactoryGraphEditorSelectionState = {
  selectedNodeIds: ReadonlySet<string>;
  selectedEdgeIds: ReadonlySet<string>;
  primaryTarget: FactoryGraphEditorSelectionTarget | null;
};

export type FactoryGraphEditorSelectionItems = {
  nodeIds?: readonly string[];
  edgeIds?: readonly string[];
  primaryTarget?: FactoryGraphEditorSelectionTarget | null;
};

export function createEmptyFactoryGraphEditorSelection(): FactoryGraphEditorSelectionState {
  return {
    selectedNodeIds: new Set(),
    selectedEdgeIds: new Set(),
    primaryTarget: null,
  };
}

function cloneSelectionSets(state: FactoryGraphEditorSelectionState): {
  selectedNodeIds: Set<string>;
  selectedEdgeIds: Set<string>;
} {
  return {
    selectedNodeIds: new Set(state.selectedNodeIds),
    selectedEdgeIds: new Set(state.selectedEdgeIds),
  };
}

function isPrimaryTargetInSelection(
  primaryTarget: FactoryGraphEditorSelectionTarget | null,
  selectedNodeIds: ReadonlySet<string>,
  selectedEdgeIds: ReadonlySet<string>,
): boolean {
  if (!primaryTarget) {
    return false;
  }

  return primaryTarget.kind === "node"
    ? selectedNodeIds.has(primaryTarget.id)
    : selectedEdgeIds.has(primaryTarget.id);
}

function resolvePrimaryTargetFromSelection(
  selectedNodeIds: ReadonlySet<string>,
  selectedEdgeIds: ReadonlySet<string>,
  preferredPrimaryTarget: FactoryGraphEditorSelectionTarget | null,
): FactoryGraphEditorSelectionTarget | null {
  if (
    preferredPrimaryTarget &&
    isPrimaryTargetInSelection(
      preferredPrimaryTarget,
      selectedNodeIds,
      selectedEdgeIds,
    )
  ) {
    return preferredPrimaryTarget;
  }

  const [firstNodeId] = selectedNodeIds;
  if (firstNodeId) {
    return { kind: "node", id: firstNodeId };
  }

  const [firstEdgeId] = selectedEdgeIds;
  if (firstEdgeId) {
    return { kind: "edge", id: firstEdgeId };
  }

  return null;
}

function resolveExplicitPrimaryTarget(
  items: FactoryGraphEditorSelectionItems,
): FactoryGraphEditorSelectionTarget | null | undefined {
  if (!("primaryTarget" in items)) {
    return undefined;
  }

  return items.primaryTarget ?? null;
}

function lastSelectionTarget(
  nodeIds: readonly string[],
  edgeIds: readonly string[],
): FactoryGraphEditorSelectionTarget | null {
  const lastEdgeId = edgeIds.at(-1);
  if (lastEdgeId) {
    return { kind: "edge", id: lastEdgeId };
  }

  const lastNodeId = nodeIds.at(-1);
  if (lastNodeId) {
    return { kind: "node", id: lastNodeId };
  }

  return null;
}

export function resolveFactoryGraphEditorPrimaryTarget(
  state: FactoryGraphEditorSelectionState,
): FactoryGraphEditorSelectionTarget | null {
  return resolvePrimaryTargetFromSelection(
    state.selectedNodeIds,
    state.selectedEdgeIds,
    state.primaryTarget,
  );
}

export function replaceFactoryGraphEditorSelection(
  _state: FactoryGraphEditorSelectionState,
  items: FactoryGraphEditorSelectionItems,
): FactoryGraphEditorSelectionState {
  const selectedNodeIds = new Set(items.nodeIds ?? []);
  const selectedEdgeIds = new Set(items.edgeIds ?? []);
  const explicitPrimaryTarget = resolveExplicitPrimaryTarget(items);
  const primaryTarget =
    explicitPrimaryTarget === undefined
      ? lastSelectionTarget([...selectedNodeIds], [...selectedEdgeIds])
      : resolvePrimaryTargetFromSelection(
          selectedNodeIds,
          selectedEdgeIds,
          explicitPrimaryTarget,
        );

  return {
    selectedNodeIds,
    selectedEdgeIds,
    primaryTarget,
  };
}

export function addToFactoryGraphEditorSelection(
  state: FactoryGraphEditorSelectionState,
  items: FactoryGraphEditorSelectionItems,
): FactoryGraphEditorSelectionState {
  const { selectedNodeIds, selectedEdgeIds } = cloneSelectionSets(state);

  for (const nodeId of items.nodeIds ?? []) {
    selectedNodeIds.add(nodeId);
  }
  for (const edgeId of items.edgeIds ?? []) {
    selectedEdgeIds.add(edgeId);
  }

  const explicitPrimaryTarget = resolveExplicitPrimaryTarget(items);
  const primaryTarget =
    explicitPrimaryTarget === undefined
      ? (lastSelectionTarget(items.nodeIds ?? [], items.edgeIds ?? []) ??
        resolvePrimaryTargetFromSelection(
          selectedNodeIds,
          selectedEdgeIds,
          state.primaryTarget,
        ))
      : resolvePrimaryTargetFromSelection(
          selectedNodeIds,
          selectedEdgeIds,
          explicitPrimaryTarget,
        );

  return {
    selectedNodeIds,
    selectedEdgeIds,
    primaryTarget,
  };
}

export function removeFromFactoryGraphEditorSelection(
  state: FactoryGraphEditorSelectionState,
  items: FactoryGraphEditorSelectionItems,
): FactoryGraphEditorSelectionState {
  const { selectedNodeIds, selectedEdgeIds } = cloneSelectionSets(state);

  for (const nodeId of items.nodeIds ?? []) {
    selectedNodeIds.delete(nodeId);
  }
  for (const edgeId of items.edgeIds ?? []) {
    selectedEdgeIds.delete(edgeId);
  }

  return {
    selectedNodeIds,
    selectedEdgeIds,
    primaryTarget: resolvePrimaryTargetFromSelection(
      selectedNodeIds,
      selectedEdgeIds,
      state.primaryTarget,
    ),
  };
}

export function clearFactoryGraphEditorSelection(): FactoryGraphEditorSelectionState {
  return createEmptyFactoryGraphEditorSelection();
}

function applyNodeSelectionChange(
  state: FactoryGraphEditorSelectionState,
  nodeId: string,
  selected: boolean,
): FactoryGraphEditorSelectionState {
  if (selected) {
    return addToFactoryGraphEditorSelection(state, {
      nodeIds: [nodeId],
    });
  }

  return removeFromFactoryGraphEditorSelection(state, {
    nodeIds: [nodeId],
  });
}

function applyEdgeSelectionChange(
  state: FactoryGraphEditorSelectionState,
  edgeId: string,
  selected: boolean,
): FactoryGraphEditorSelectionState {
  if (selected) {
    return addToFactoryGraphEditorSelection(state, {
      edgeIds: [edgeId],
    });
  }

  return removeFromFactoryGraphEditorSelection(state, {
    edgeIds: [edgeId],
  });
}

export function applyFactoryGraphEditorEdgeSelectChanges(
  state: FactoryGraphEditorSelectionState,
  changes: readonly EdgeChange[],
): FactoryGraphEditorSelectionState {
  let nextState = state;

  for (const change of changes) {
    if (
      change.type !== "select" &&
      change.type !== "remove" &&
      change.type !== "add" &&
      change.type !== "replace"
    ) {
      continue;
    }

    if (change.type === "select") {
      nextState = applyEdgeSelectionChange(
        nextState,
        change.id,
        change.selected,
      );
      continue;
    }

    if (change.type === "remove") {
      nextState = removeFromFactoryGraphEditorSelection(nextState, {
        edgeIds: [change.id],
      });
      continue;
    }

    const selected = change.item.selected === true;
    nextState = applyEdgeSelectionChange(nextState, change.item.id, selected);
  }

  return nextState;
}

export function applyFactoryGraphEditorNodeSelectChanges(
  state: FactoryGraphEditorSelectionState,
  changes: readonly NodeChange[],
): FactoryGraphEditorSelectionState {
  let nextState = state;

  for (const change of changes) {
    if (
      change.type !== "select" &&
      change.type !== "remove" &&
      change.type !== "add" &&
      change.type !== "replace"
    ) {
      continue;
    }

    if (change.type === "select") {
      nextState = applyNodeSelectionChange(
        nextState,
        change.id,
        change.selected,
      );
      continue;
    }

    if (change.type === "remove") {
      nextState = removeFromFactoryGraphEditorSelection(nextState, {
        nodeIds: [change.id],
      });
      continue;
    }

    const selected = change.item.selected === true;
    nextState = applyNodeSelectionChange(nextState, change.item.id, selected);
  }

  return nextState;
}
