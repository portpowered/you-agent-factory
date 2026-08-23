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
  contentInset?: GraphNodeContentInset;
  handles: GraphNodeHandle[];
  nodeKind?: string;
  showStateIndicator?: boolean;
  state?: GraphNodeState;
  stateLabel?: string;
}

export type GraphNodeContentInset = "compact" | "default";

export function GraphNodeShell({
  children,
  className = "",
  contentInset = "default",
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
          graphNodeContentInsetClassName(
            contentInset,
            leftHandles.length > 0,
            rightHandles.length > 0,
          ),
        )}
        data-graph-node-content
        data-graph-node-content-inset={contentInset}
      >
        {showStateIndicator ? (
          <GraphNodeStateIndicator state={state} stateLabel={stateLabel} />
        ) : null}
        {children}
      </div>
    </article>
  );
}

function graphNodeContentInsetClassName(
  contentInset: GraphNodeContentInset,
  hasLeftRail: boolean,
  hasRightRail: boolean,
): string {
  if (contentInset === "compact") {
    if (hasLeftRail && hasRightRail) return "pl-5 pr-5";
    if (hasLeftRail) return "pl-5 pr-2";
    if (hasRightRail) return "pl-2 pr-5";
    return "px-2";
  }

  if (hasLeftRail && hasRightRail) return "pl-6 pr-6";
  if (hasLeftRail) return "pl-6 pr-3";
  if (hasRightRail) return "pl-3 pr-6";
  return "px-3";
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
            side === "left" ? "left-0" : "right-0",
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
