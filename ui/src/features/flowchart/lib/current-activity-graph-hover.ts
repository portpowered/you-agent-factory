export interface CurrentActivityGraphNodeHoverState {
  activeFlow?: boolean;
  muted?: boolean;
  selected?: boolean;
  validationError?: boolean;
}

/** Accent border/shadow applied on pointer hover when higher-priority states are absent. */
export const CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS =
  "transition-[border-color,box-shadow] hover:border-af-accent-border hover:shadow-af-accent-chip";

export function currentActivityGraphNodeHoverClassName(
  state: CurrentActivityGraphNodeHoverState,
): string | undefined {
  if (state.muted || state.selected || state.validationError || state.activeFlow) {
    return undefined;
  }

  return CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS;
}

export interface CurrentActivityGraphEdgeHoverState {
  active?: boolean;
  activeFlow?: boolean;
  muted?: boolean;
  pendingAddition?: boolean;
  pendingRemoval?: boolean;
  semantic?: boolean;
}

export const CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS =
  "agent-flow-edge--hoverable";

export const FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS =
  "agent-factory-editor-edge--hoverable";

export function currentActivityGraphEdgeHoverClassName(
  state: CurrentActivityGraphEdgeHoverState,
): string | undefined {
  if (
    state.active ||
    state.activeFlow ||
    state.muted ||
    state.semantic ||
    state.pendingAddition ||
    state.pendingRemoval
  ) {
    return undefined;
  }

  return CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS;
}

export function factoryGraphEditorEdgeHoverClassName(
  state: CurrentActivityGraphEdgeHoverState,
): string | undefined {
  if (
    state.active ||
    state.activeFlow ||
    state.pendingAddition ||
    state.pendingRemoval
  ) {
    return undefined;
  }

  return FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS;
}
