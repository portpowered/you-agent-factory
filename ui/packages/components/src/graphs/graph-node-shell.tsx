import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../utilities/cn";
import type { GraphNodeHandle } from "./graph-node-handle";
import { GraphNodeHandleBadge } from "./graph-node-handle-badge";
import {
  GRAPH_NODE_CONTENT_MIN_HEIGHT_CLASS,
  type GraphNodeState,
  graphNodeShellStateAttributes,
  graphNodeShellStateClassName,
} from "./graph-node-state";
import { GraphNodeStateIndicator } from "./graph-node-state-indicator";

export interface GraphNodeShellProps
  extends Omit<HTMLAttributes<HTMLElement>, "children"> {
  children: ReactNode;
  className?: string;
  handles: GraphNodeHandle[];
  nodeKind?: string;
  showStateIndicator?: boolean;
  state?: GraphNodeState;
  stateLabel?: string;
}

export function GraphNodeShell({
  children,
  className = "",
  handles,
  nodeKind,
  showStateIndicator = true,
  state = "default",
  stateLabel,
  ...articleProps
}: GraphNodeShellProps) {
  const leftHandles = handles.filter((handle) => handle.side === "left");
  const rightHandles = handles.filter((handle) => handle.side === "right");

  return (
    <article
      className={cn(
        "relative flex h-full min-w-0 w-full overflow-visible rounded-lg border border-outline bg-surface text-on-surface",
        graphNodeShellStateClassName(state),
        className,
      )}
      data-graph-node-kind={nodeKind}
      {...graphNodeShellStateAttributes(state, stateLabel)}
      {...articleProps}
    >
      <NodeHandleRail handles={leftHandles} side="left" />
      <NodeHandleRail handles={rightHandles} side="right" />
      <div
        className={cn(
          "flex h-full min-w-0 w-full flex-col gap-1 py-3",
          showStateIndicator && GRAPH_NODE_CONTENT_MIN_HEIGHT_CLASS,
          leftHandles.length > 0 ? "pl-6 pr-3" : "px-3",
          rightHandles.length > 0 && leftHandles.length > 0
            ? "pr-6"
            : rightHandles.length > 0
              ? "pl-3 pr-6"
              : null,
        )}
      >
        {showStateIndicator ? (
          <GraphNodeStateIndicator state={state} stateLabel={stateLabel} />
        ) : null}
        {children}
      </div>
    </article>
  );
}

function NodeHandleRail({
  handles,
  side,
}: {
  handles: GraphNodeHandle[];
  side: "left" | "right";
}) {
  if (handles.length === 0) {
    return null;
  }

  return (
    <div
      className={cn(
        "pointer-events-none absolute inset-y-0 z-20 w-6",
        side === "left" ? "left-0" : "right-0",
      )}
      data-node-handle-rail={side}
    >
      {handles.map((handle, index) => (
        <div
          className={cn(
            "absolute top-0 flex -translate-y-1/2",
            side === "left"
              ? "left-0 -translate-x-1/2"
              : "right-0 translate-x-1/2",
          )}
          key={handle.id}
          style={{ top: handlePosition(index, handles.length) }}
        >
          <GraphNodeHandleBadge handle={handle} />
        </div>
      ))}
    </div>
  );
}

function handlePosition(index: number, count: number): string {
  return `${((index + 1) * 100) / (count + 1)}%`;
}
