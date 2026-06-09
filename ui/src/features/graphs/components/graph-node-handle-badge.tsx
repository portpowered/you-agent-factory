import { Handle, Position } from "@xyflow/react";
import type { CSSProperties } from "react";

import { cn } from "../../../lib/cn";
import type { ActivityGraphNodeHandle } from "./graph-node-shell";

interface GraphNodeHandleBadgeProps {
  handle: ActivityGraphNodeHandle;
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

  const tone = handleTone(handle.id);

  return (
    <div
      className="pointer-events-none relative flex h-5 w-5 items-center justify-center"
      data-node-handle-badge={handle.id}
      data-node-handle-invalid={handle.validationError ? "true" : undefined}
      data-node-handle-tone={tone}
    >
      <Handle
        aria-invalid={handle.validationError ? true : undefined}
        aria-label={handle.buttonAriaLabel}
        className="pointer-events-auto absolute !top-1/2 !h-full !w-full !border-0 opacity-0"
        id={handle.id}
        isConnectable={handle.connectable ?? true}
        onClick={handle.onButtonClick}
        position={position}
        style={overlayHandleStyle}
        title={handle.buttonTitle ?? handle.validationMessage}
        type={handle.type}
      />
      <span
        aria-hidden="true"
        className={cn(
          "block h-2.5 w-2.5 rounded-full border border-surface shadow-sm transition",
          handle.buttonPressed &&
            "scale-125 shadow-[0_0_0_3px_var(--color-primary-container)]",
          handle.variant === "valid-target" &&
            "scale-125 shadow-[0_0_0_3px_var(--color-success-container)]",
          handle.variant === "error" &&
            "border-af-danger-border shadow-[0_0_0_3px_var(--color-error-container)] motion-safe:animate-pulse",
          handle.validationError &&
            "ring-2 ring-af-danger-border motion-safe:animate-pulse",
        )}
        style={{
          ...handleDotStyle(tone),
          opacity: handle.buttonDisabled ? 0.45 : undefined,
        }}
      />
    </div>
  );
}

function handleTone(
  handleId: string,
):
  | "assignment"
  | "continue"
  | "default"
  | "failure"
  | "input"
  | "output"
  | "worker"
  | "rejection"
  | "resource" {
  if (handleId.includes("resource")) {
    return "resource";
  }

  if (
    handleId.includes("worker-assignment") ||
    handleId.includes("worker-input")
  ) {
    return "worker";
  }

  if (handleId.includes("on-continue")) {
    return "continue";
  }

  if (handleId.includes("on-failure")) {
    return "failure";
  }

  if (handleId.includes("on-rejection")) {
    return "rejection";
  }

  if (handleId.includes("output")) {
    return "output";
  }

  if (handleId.includes("input")) {
    return "input";
  }

  if (handleId.includes("assignment")) {
    return "assignment";
  }

  return "default";
}

function handleDotStyle(tone: ReturnType<typeof handleTone>): CSSProperties {
  switch (tone) {
    case "assignment":
      return semanticDotStyle("var(--color-success)");
    case "continue":
      return semanticDotStyle("var(--color-secondary)");
    case "failure":
      return semanticDotStyle("var(--color-error)");
    case "input":
      return semanticDotStyle("var(--color-success)");
    case "output":
      return semanticDotStyle("var(--color-success)");
    case "rejection":
      return semanticDotStyle("var(--color-warning)");
    case "resource":
      return semanticDotStyle("var(--color-black)");
    case "worker":
      return semanticDotStyle("var(--color-purple-500)");
    default:
      return semanticDotStyle("var(--color-success)");
  }
}

function semanticDotStyle(backgroundColor: string): CSSProperties {
  return {
    backgroundColor,
    borderColor: "var(--color-surface)",
  };
}

function anchoredHandleStyle(
  side: ActivityGraphNodeHandle["side"],
): CSSProperties {
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
