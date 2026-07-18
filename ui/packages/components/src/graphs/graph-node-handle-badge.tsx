import { Handle, Position } from "@xyflow/react";
import type { CSSProperties } from "react";

import { cn } from "../utilities/cn";
import type { GraphNodeHandle, GraphNodeHandleTone } from "./graph-node-handle";

export interface GraphNodeHandleBadgeProps {
  handle: GraphNodeHandle;
}

export function GraphNodeHandleBadge({ handle }: GraphNodeHandleBadgeProps) {
  const position = handle.side === "left" ? Position.Left : Position.Right;
  const overlayHandleStyle = anchoredHandleStyle(handle.side);

  if (handle.hidden) {
    return (
      <Handle
        className="pointer-events-none !top-1/2 opacity-0"
        id={handle.id}
        isConnectable={handle.connectable ?? false}
        position={position}
        style={overlayHandleStyle}
        type={handle.type}
      />
    );
  }

  const tone = handle.tone ?? "default";

  return (
    <div
      className="pointer-events-none relative flex h-5 w-5 items-center justify-center"
      data-node-handle-badge={handle.id}
      data-node-handle-invalid={handle.validationError ? "true" : undefined}
      data-node-handle-tone={tone}
    >
      <Handle
        aria-invalid={handle.validationError ? true : undefined}
        aria-label={handle.buttonAriaLabel ?? handle.label}
        className={cn(
          "pointer-events-auto absolute !top-1/2 !h-5 !w-5 !border-0 !bg-transparent",
          "before:pointer-events-none before:absolute before:top-1/2 before:h-2.5 before:w-2.5 before:-translate-x-1/2 before:-translate-y-1/2 before:rounded-full before:border before:border-surface before:bg-[var(--node-handle-background)] before:shadow-sm before:transition before:content-['']",
          handle.side === "left" ? "before:left-0" : "before:left-full",
          handle.buttonPressed &&
            "before:scale-125 before:shadow-[0_0_0_3px_var(--color-primary-container)]",
          handle.variant === "valid-target" &&
            "before:scale-125 before:shadow-[0_0_0_3px_var(--color-success-container)]",
          handle.variant === "error" &&
            "before:border-af-danger-border before:shadow-[0_0_0_3px_var(--color-error-container)] motion-safe:before:animate-pulse",
          handle.validationError &&
            "before:ring-2 before:ring-af-danger-border motion-safe:before:animate-pulse",
        )}
        id={handle.id}
        isConnectable={handle.connectable ?? true}
        onClick={handle.onButtonClick}
        position={position}
        role="img"
        style={
          {
            ...overlayHandleStyle,
            "--node-handle-background": handleDotColor(tone),
            opacity: handle.buttonDisabled ? 0.45 : undefined,
          } as CSSProperties
        }
        title={handle.buttonTitle ?? handle.validationMessage}
        type={handle.type}
      />
    </div>
  );
}

function handleDotColor(tone: GraphNodeHandleTone): string {
  switch (tone) {
    case "assignment":
      return "var(--color-success)";
    case "continue":
      return "var(--color-secondary)";
    case "failure":
      return "var(--color-error)";
    case "input":
      return "var(--color-success)";
    case "output":
      return "var(--color-success)";
    case "rejection":
      return "var(--color-warning)";
    case "resource":
      return "var(--color-black)";
    case "worker":
      return "var(--color-purple-500)";
    default:
      return "var(--color-success)";
  }
}

function anchoredHandleStyle(side: GraphNodeHandle["side"]): CSSProperties {
  return side === "left"
    ? {
        left: "50%",
        top: "50%",
        transform: "translateY(-50%)",
      }
    : {
        left: "50%",
        top: "50%",
        transform: "translate(-100%, -50%)",
      };
}
