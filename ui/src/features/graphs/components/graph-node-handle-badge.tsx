import { Handle, Position } from "@xyflow/react";
import type { CSSProperties } from "react";

import { cn } from "../../../lib/cn";
import { GraphNodeButton } from "./graph-node-button";
import type { ActivityGraphNodeHandle } from "./graph-node-shell";

interface GraphNodeHandleBadgeProps {
  handle: ActivityGraphNodeHandle;
}

export function GraphNodeHandleBadge({ handle }: GraphNodeHandleBadgeProps) {
  const position = handle.side === "left" ? Position.Left : Position.Right;

  if (handle.hidden) {
    return (
      <Handle
        className="pointer-events-none opacity-0"
        id={handle.id}
        isConnectable={handle.connectable ?? false}
        position={position}
        type={handle.type}
      />
    );
  }

  const dotClassName = handleDotClassName(handle);
  const dotStyle = handleDotStyle(handle);

  return (
    <div
      className={cn(
        "pointer-events-none relative flex h-5 w-5 items-center justify-center",
        handle.side === "left" ? "self-start" : "self-end",
      )}
      data-node-handle-badge={handle.id}
    >
      <Handle
        className="pointer-events-auto absolute inset-0 !h-full !w-full !border-0 opacity-0"
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
          "pointer-events-auto -m-1 grid h-5 w-5 place-items-center rounded-full transition focus-visible:outline-2 focus-visible:outline-af-focus-ring disabled:cursor-not-allowed disabled:bg-surface-container-low disabled:text-on-surface-disabled",
          handle.validationError &&
            "ring-2 ring-af-danger-border motion-safe:animate-pulse",
        )}
        disabled={handle.buttonDisabled}
        onClick={handle.onButtonClick}
        title={handle.buttonTitle ?? handle.validationMessage}
      >
        <span
          aria-hidden="true"
          className={cn(
            "block h-2.5 w-2.5 rounded-full border border-surface shadow-sm transition",
            dotClassName,
            handle.variant === "selected" &&
              "scale-125 shadow-[0_0_0_3px_var(--color-primary-container)]",
            handle.variant === "valid-target" &&
              "scale-125 shadow-[0_0_0_3px_var(--color-success-container)]",
            handle.variant === "error" &&
              "scale-125 border-af-danger-border bg-error-container shadow-[0_0_0_3px_var(--color-error-container)] motion-safe:animate-pulse",
          )}
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
      backgroundColor: "var(--color-on-inverse)",
      borderColor: "var(--color-on-surface)",
    };
  }

  return undefined;
}

function handleDotClassName(handle: ActivityGraphNodeHandle): string {
  if (handle.variant === "error") {
    return "bg-error";
  }

  if (handle.variant === "muted") {
    return "bg-outline-variant";
  }

  if (
    handle.id === "workstation-resource-source" ||
    handle.id === "workstation-resource-target"
  ) {
    return "bg-on-inverse";
  }

  if (handle.variant === "selected") {
    return "bg-primary";
  }

  if (handle.variant === "valid-target") {
    return "bg-success";
  }

  return "bg-on-surface";
}
