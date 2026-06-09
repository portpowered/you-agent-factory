import type { ReactNode } from "react";

import { cn } from "../../../lib/cn";
import { GraphNodeHandleBadge } from "./graph-node-handle-badge";

export type PlaceNodeType =
  | "constraint"
  | "doc"
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

export interface ZAxisIncompleteHints {
  accessibleLabel: string;
  title: string;
}

const WORKSTATION_RIGHT_RAIL_ANCHOR_IDS = [
  "workstation-output-source",
  "workstation-on-continue-source",
  "workstation-on-failure-source",
  "workstation-on-rejection-source",
] as const;

const Z_AXIS_INCOMPLETE_HINT_ANCHOR_IDS = [
  "workstation-on-continue-source",
  "workstation-on-rejection-source",
] as const;

interface ActivityGraphNodeShellProps {
  children: ReactNode;
  className?: string;
  handles: ActivityGraphNodeHandle[];
  nodeType: "workstation" | PlaceNodeType;
  zAxisIncompleteHints?: ZAxisIncompleteHints | null;
}

export function ActivityGraphNodeShell({
  children,
  className = "",
  handles,
  nodeType,
  zAxisIncompleteHints = null,
}: ActivityGraphNodeShellProps) {
  const leftHandles = handles.filter((handle) => handle.side === "left");
  const rightHandles = handles.filter((handle) => handle.side === "right");
  const activeZAxisIncompleteHints =
    nodeType === "workstation" ? zAxisIncompleteHints : null;
  const zAxisHintSlots = activeZAxisIncompleteHints
    ? workstationZAxisIncompleteHintSlots()
    : [];

  return (
    <article
      className={cn(
        "relative flex h-full min-w-0 w-full overflow-visible rounded-lg border border-outline bg-surface text-on-surface",
        className,
      )}
      data-current-activity-node-type={nodeType}
    >
      <NodeHandleRail handles={leftHandles} side="left" />
      <NodeHandleRail handles={rightHandles} side="right" />
      {activeZAxisIncompleteHints
        ? zAxisHintSlots.map((slot) => (
            <ZAxisIncompleteHintOrb
              accessibleLabel={activeZAxisIncompleteHints.accessibleLabel}
              anchorId={slot.anchorId}
              key={slot.anchorId}
              title={activeZAxisIncompleteHints.title}
              top={slot.top}
            />
          ))
        : null}
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
  handles: ActivityGraphNodeHandle[];
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

function workstationZAxisIncompleteHintSlots(): Array<{
  anchorId: (typeof Z_AXIS_INCOMPLETE_HINT_ANCHOR_IDS)[number];
  top: string;
}> {
  const railCount = WORKSTATION_RIGHT_RAIL_ANCHOR_IDS.length;

  return Z_AXIS_INCOMPLETE_HINT_ANCHOR_IDS.map((anchorId) => {
    const index = WORKSTATION_RIGHT_RAIL_ANCHOR_IDS.indexOf(anchorId);

    return {
      anchorId,
      top: handlePosition(index, railCount),
    };
  });
}

function handlePosition(index: number, count: number): string {
  return `${((index + 1) * 100) / (count + 1)}%`;
}

function ZAxisIncompleteHintOrb({
  accessibleLabel,
  anchorId,
  title,
  top,
}: {
  accessibleLabel: string;
  anchorId: string;
  title: string;
  top: string;
}) {
  return (
    <span
      aria-label={accessibleLabel}
      className="pointer-events-none absolute top-0 right-0 z-20 flex -translate-y-1/2 translate-x-1 flex-row-reverse"
      data-z-axis-incomplete-hint={anchorId}
      role="img"
      style={{ top }}
      title={title}
    >
      <span
        aria-hidden="true"
        className="block h-2.5 w-2.5 rounded-full border border-af-danger-border bg-error shadow-[0_0_0_3px_var(--color-error-container)] motion-safe:animate-pulse"
      />
    </span>
  );
}
