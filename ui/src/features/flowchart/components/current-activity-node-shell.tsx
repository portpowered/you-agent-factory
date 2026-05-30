import { Handle, Position } from "@xyflow/react";
import type { CSSProperties, ReactNode } from "react";

import { GraphNodeButton } from "../../../components/ui/graph-node-button";
import { cn } from "../../../lib/cn";

export type PlaceNodeType =
  | "constraint"
  | "resource"
  | "statePosition"
  | "worker"
  | "workType";

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
  validationError?: boolean;
  validationMessage?: string;
  variant?: "default" | "error" | "muted" | "selected" | "valid-target";
}

interface ActivityGraphNodeShellProps {
  children: ReactNode;
  className?: string;
  handles: ActivityGraphNodeHandle[];
  nodeType: "workstation" | PlaceNodeType;
}

export function ActivityGraphNodeShell({
  children,
  className = "",
  handles,
  nodeType,
}: ActivityGraphNodeShellProps) {
  const leftHandles = handles.filter((handle) => handle.side === "left");
  const rightHandles = handles.filter((handle) => handle.side === "right");

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
      {rightHandles.map((handle, handleNumber) => (
        <NodeHandleBadge
          handle={handle}
          key={handle.id}
          top={handlePosition(handleNumber, rightHandles.length)}
        />
      ))}
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
  const dotClassName = handleDotClassName(handle);
  const dotStyle = handleDotStyle(handle);

  return (
    <div
      className={cn(
        "pointer-events-none absolute top-0 z-20 flex -translate-y-1/2 items-center",
        handle.side === "left" ? "left-0" : "right-0",
        wrapperClassName,
      )}
      style={{ top }}
    >
      <Handle
        className={cn("pointer-events-auto !h-2.5 !w-2.5 !border-0 opacity-0")}
        id={handle.id}
        isConnectable={handle.connectable ?? true}
        position={position}
        type={handle.type}
      />
      <GraphNodeButton
        aria-invalid={handle.validationError ? true : undefined}
        aria-label={handle.buttonAriaLabel}
        aria-pressed={handle.buttonPressed}
        className={cn(
          "pointer-events-auto -m-1 grid h-5 w-5 place-items-center rounded-full transition focus-visible:outline-2 focus-visible:outline-af-focus-ring disabled:cursor-not-allowed disabled:bg-af-surface-subtle disabled:text-af-text-disabled",
          handle.validationError &&
            "ring-2 ring-af-danger-border motion-safe:animate-pulse",
        )}
        disabled={handle.buttonDisabled}
        onClick={handle.onButtonClick}
        title={handle.buttonTitle ?? handle.validationMessage}
      >
        <span
          aria-hidden={handle.validationError ? undefined : "true"}
          aria-label={
            handle.validationError ? handle.validationMessage : undefined
          }
          className={cn(
            "block h-2.5 w-2.5 rounded-full border border-af-surface shadow-sm transition",
            dotClassName,
            handle.variant === "selected" &&
              "scale-125 shadow-[0_0_0_3px_var(--color-af-accent-surface)]",
            handle.variant === "valid-target" &&
              "scale-125 shadow-[0_0_0_3px_var(--color-af-success-surface)]",
            handle.variant === "error" &&
              "scale-125 border-af-danger-border bg-af-danger-surface shadow-[0_0_0_3px_var(--color-af-danger-surface)] motion-safe:animate-pulse",
          )}
          role={handle.validationError ? "img" : undefined}
          style={dotStyle}
        />
      </GraphNodeButton>
    </div>
  );
}

function handleDotStyle(
  handle: ActivityGraphNodeHandle,
): CSSProperties | undefined {
  if (
    handle.id === "workstation-resource-source" ||
    handle.id === "workstation-resource-target"
  ) {
    return {
      backgroundColor: "var(--color-af-text-inverse)",
      borderColor: "var(--color-af-text)",
    };
  }

  return undefined;
}

function handleDotClassName(handle: ActivityGraphNodeHandle): string {
  if (handle.variant === "error") {
    return "bg-af-danger";
  }

  if (handle.variant === "muted") {
    return "bg-af-border-strong";
  }

  if (
    handle.id === "workstation-resource-source" ||
    handle.id === "workstation-resource-target"
  ) {
    return "";
  }
  if (handle.id.includes("on-continue")) {
    return "bg-af-info";
  }
  if (handle.id.includes("on-failure")) {
    return "bg-af-danger";
  }
  if (handle.id.includes("on-rejection")) {
    return "bg-af-warning";
  }
  if (handle.id.includes("worker")) {
    return "bg-af-worker";
  }
  if (
    handle.id.includes("input") ||
    handle.id.includes("output") ||
    handle.id.includes("resource")
  ) {
    return "bg-af-success";
  }

  return "bg-af-border-strong";
}
