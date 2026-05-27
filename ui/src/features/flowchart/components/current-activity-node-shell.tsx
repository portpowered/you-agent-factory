import { Handle, Position } from "@xyflow/react";
import type { ReactNode } from "react";

import { cn } from "../../../lib/cn";

export type PlaceNodeType = "constraint" | "resource" | "statePosition";

export interface ActivityGraphNodeHandle {
  buttonAriaLabel?: string;
  buttonDisabled?: boolean;
  buttonPressed?: boolean;
  buttonTitle?: string;
  connectable?: boolean;
  hidden?: boolean;
  id: string;
  label: string;
  onButtonClick?: () => void;
  side: "left" | "right";
  type: "source" | "target";
  variant?: "default" | "muted" | "selected" | "valid-target";
}

interface ActivityGraphNodeShellProps {
  children: ReactNode;
  className?: string;
  handles?: ActivityGraphNodeHandle[];
  incomingHandleCount: number;
  nodeType: "workstation" | PlaceNodeType;
  outgoingHandleCount: number;
}

export function ActivityGraphNodeShell({
  children,
  className = "",
  handles,
  incomingHandleCount,
  nodeType,
  outgoingHandleCount,
}: ActivityGraphNodeShellProps) {
  const leftHandles = handles?.filter((handle) => handle.side === "left") ?? [];
  const rightHandles =
    handles?.filter((handle) => handle.side === "right") ?? [];
  const renderPositionalHandles = handles === undefined;

  return (
    <article
      className={cn(
        "flex h-full min-w-0 w-full flex-col gap-1 overflow-visible rounded-lg border border-af-border bg-af-surface p-3 text-af-text",
        className,
      )}
      data-current-activity-node-type={nodeType}
    >
      {leftHandles.map((handle, handleNumber) => (
        <NodeHandleBadge
          handle={handle}
          key={handle.id}
          top={handlePosition(handleNumber, leftHandles.length)}
        />
      ))}
      {renderPositionalHandles &&
        Array.from({ length: incomingHandleCount }, (_, handleNumber) => {
          const top = handlePosition(handleNumber, incomingHandleCount);
          const handleId = `in-${handleNumber}`;
          return (
            <Handle
              className="pointer-events-none opacity-0"
              id={handleId}
              key={`incoming-${top}`}
              position={Position.Left}
              style={{ top }}
              type="target"
            />
          );
        })}
      {rightHandles.map((handle, handleNumber) => (
        <NodeHandleBadge
          handle={handle}
          key={handle.id}
          top={handlePosition(handleNumber, rightHandles.length)}
        />
      ))}
      {renderPositionalHandles &&
        Array.from({ length: outgoingHandleCount }, (_, handleNumber) => {
          const top = handlePosition(handleNumber, outgoingHandleCount);
          const handleId = `out-${handleNumber}`;
          return (
            <Handle
              className="pointer-events-none opacity-0"
              id={handleId}
              key={`outgoing-${top}`}
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

function NodeHandleBadge({
  handle,
  top,
}: {
  handle: ActivityGraphNodeHandle;
  top: string;
}) {
  const position = handle.side === "left" ? Position.Left : Position.Right;
  if (handle.hidden) {
    return (
      <Handle
        className="pointer-events-none opacity-0"
        id={handle.id}
        isConnectable={handle.connectable ?? false}
        position={position}
        style={{ top }}
        type={handle.type}
      />
    );
  }
  const wrapperClassName =
    handle.side === "left"
      ? "-translate-x-1 flex-row"
      : "translate-x-1 flex-row-reverse";
  const buttonClassName = cn(
    "nodrag nopan inline-flex min-h-6 items-center rounded-full border px-2 py-1 text-[0.625rem] font-semibold uppercase tracking-[0.08em] shadow-sm transition focus-visible:outline-2 focus-visible:outline-af-accent disabled:cursor-not-allowed",
    handle.variant === "selected" &&
      "border-af-accent-border bg-af-accent-surface text-af-accent",
    handle.variant === "valid-target" &&
      "border-af-success-border bg-af-success-surface text-af-success",
    handle.variant === "muted" &&
      "border-af-border bg-af-surface-subtle text-af-text-subtle",
    (handle.variant === undefined || handle.variant === "default") &&
      "border-af-border-strong bg-af-surface-raised text-af-text-muted hover:border-af-accent-border hover:text-af-text",
  );

  return (
    <div
      className={cn(
        "pointer-events-none absolute top-0 z-20 flex -translate-y-1/2 items-center gap-1.5",
        handle.side === "left" ? "left-0" : "right-0",
        wrapperClassName,
      )}
      style={{ top }}
    >
      <Handle
        className={cn(
          "pointer-events-auto !h-3.5 !w-3.5 !border-2 !border-af-surface !bg-af-border-strong transition",
          handle.connectable && "!bg-af-accent-border",
          handle.variant === "selected" && "!bg-af-accent",
          handle.variant === "valid-target" && "!bg-af-success",
        )}
        id={handle.id}
        isConnectable={handle.connectable ?? true}
        position={position}
        type={handle.type}
      />
      <button
        aria-label={handle.buttonAriaLabel}
        aria-pressed={handle.buttonPressed}
        className={cn("pointer-events-auto", buttonClassName)}
        disabled={handle.buttonDisabled}
        onClick={handle.onButtonClick}
        title={handle.buttonTitle}
        type="button"
      >
        {handle.label}
      </button>
    </div>
  );
}
