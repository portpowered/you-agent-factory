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
export {
  type GraphNodeHandle,
  type GraphNodeHandleTone,
} from "./graph-node-handle";
export {
  GraphNodeHandleBadge,
  type GraphNodeHandleBadgeProps,
} from "./graph-node-handle-badge";
export { GraphNodeShell, type GraphNodeShellProps } from "./graph-node-shell";
export {
  buildGraphEdgePathThroughWaypoints,
  type GraphEdgeWaypoint,
} from "./graph-edge-path";
export {
  GRAPH_EDGE_TYPES,
  GraphEdge,
  type GraphEdgeData,
} from "./graph-edge";
