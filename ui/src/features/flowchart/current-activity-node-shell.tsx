import { Handle, Position } from "@xyflow/react";
import type { ReactNode } from "react";

import { cx } from "../../lib/cx";

export type PlaceNodeType = "constraint" | "resource" | "statePosition";

interface ActivityGraphNodeShellProps {
  children: ReactNode;
  className?: string;
  incomingHandleCount: number;
  nodeType: "workstation" | PlaceNodeType;
  outgoingHandleCount: number;
}

export function ActivityGraphNodeShell({
  children,
  className = "",
  incomingHandleCount,
  nodeType,
  outgoingHandleCount,
}: ActivityGraphNodeShellProps) {
  return (
    <article
      className={cx(
        "flex h-full min-w-0 w-full flex-col gap-[0.35rem] overflow-hidden rounded-lg border border-af-overlay/9 bg-af-canvas p-[0.75rem] text-af-ink",
        className,
      )}
      data-current-activity-node-type={nodeType}
    >
      {Array.from({ length: incomingHandleCount }, (_, handleNumber) => {
        const top = handlePosition(handleNumber, incomingHandleCount);
        return (
          <Handle
            className="opacity-0"
            id={`in-${top}`}
            key={`in-${top}`}
            position={Position.Left}
            style={{ top }}
            type="target"
          />
        );
      })}
      {Array.from({ length: outgoingHandleCount }, (_, handleNumber) => {
        const top = handlePosition(handleNumber, outgoingHandleCount);
        return (
          <Handle
            className="opacity-0"
            id={`out-${top}`}
            key={`out-${top}`}
            position={Position.Right}
            style={{ top }}
            type="source"
          />
        );
      })}
      {children}
    </article>
  );
}

function handlePosition(index: number, count: number): string {
  return `${((index + 1) * 100) / (count + 1)}%`;
}
