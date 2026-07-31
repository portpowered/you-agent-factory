import {
  factoryGraphNodeHoverClassName,
  type FactoryGraphNodeHoverState,
  type FactoryGraphNodeHoverSurface,
} from "@you-agent-factory/factory-graph";

export type CurrentActivityGraphNodeHoverState = FactoryGraphNodeHoverState;

export type CurrentActivityGraphNodeHoverSurface = FactoryGraphNodeHoverSurface;

export function currentActivityGraphNodeHoverClassName(
  state: CurrentActivityGraphNodeHoverState,
  surface: CurrentActivityGraphNodeHoverSurface = "warning",
): string | undefined {
  return factoryGraphNodeHoverClassName(state, surface);
}

export interface CurrentActivityGraphEdgeHoverState {
  active?: boolean;
  activeFlow?: boolean;
  muted?: boolean;
  pendingAddition?: boolean;
  pendingRemoval?: boolean;
  semantic?: boolean;
}

const CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS = "agent-flow-edge--hoverable";

const FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS =
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
