import type { ReactNode } from "react";

import { cn } from "../utilities/cn";
import { GraphNodeHandleBadge } from "./graph-node-handle-badge";
import type { GraphNodeHandle } from "./graph-node-handle";

export interface GraphNodeShellProps {
  children: ReactNode;
  className?: string;
  handles: GraphNodeHandle[];
  nodeKind?: string;
}

export function GraphNodeShell({
  children,
  className = "",
  handles,
  nodeKind,
}: GraphNodeShellProps) {
  const leftHandles = handles.filter((handle) => handle.side === "left");
  const rightHandles = handles.filter((handle) => handle.side === "right");

  return (
    <article
      className={cn(
        "relative flex h-full min-w-0 w-full overflow-visible rounded-lg border border-outline bg-surface text-on-surface",
        className,
      )}
      data-graph-node-kind={nodeKind}
    >
      <NodeHandleRail handles={leftHandles} side="left" />
      <NodeHandleRail handles={rightHandles} side="right" />
      <div
        className={cn(
          "flex h-full min-w-0 w-full flex-col gap-1 py-3",
          leftHandles.length > 0 ? "pl-6 pr-3" : "px-3",
          rightHandles.length > 0 && leftHandles.length > 0
            ? "pr-6"
            : rightHandles.length > 0
              ? "pl-3 pr-6"
              : null,
        )}
      >
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
