/** Stable category path for `@you-agent-factory/components/graphs`. */
export const COMPONENTS_CATEGORY = "graphs" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export {
  GRAPH_NODE_BUTTON_BASE_CLASS,
  GraphNodeButton,
  type GraphNodeButtonProps,
} from "./graph-node-button";
export {
  GraphViewportSurface,
  type GraphViewportSurfaceProps,
} from "./graph-viewport-surface";
export type { GraphNodeHandle, GraphNodeHandleTone } from "./graph-node-handle";
export {
  GraphNodeHandleBadge,
  type GraphNodeHandleBadgeProps,
} from "./graph-node-handle-badge";
export { GraphNodeShell, type GraphNodeShellProps } from "./graph-node-shell";
export {
  defaultGraphNodeStateLabel,
  GRAPH_NODE_CONTENT_MIN_HEIGHT_CLASS,
  GRAPH_NODE_STATE_INDICATOR_HEIGHT_CLASS,
  graphNodeButtonIsDisabled,
  graphNodeButtonStateAttributes,
  graphNodeButtonStateClassName,
  graphNodeShellStateAttributes,
  graphNodeShellStateClassName,
  type GraphNodeState,
} from "./graph-node-state";
export {
  GraphNodeStateIndicator,
  type GraphNodeStateIndicatorProps,
} from "./graph-node-state-indicator";
export {
  buildGraphEdgePathThroughWaypoints,
  type GraphEdgeWaypoint,
} from "./graph-edge-path";
export type { GraphEdgeData, GraphEdgeProps } from "./graph-edge";
export { GRAPH_EDGE_TYPES, GraphEdge } from "./graph-edge";
