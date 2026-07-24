import {
  type GraphNodeHandle,
  GraphNodeShell,
} from "@you-agent-factory/components/graphs";
import type { ReactNode } from "react";

import { cn } from "../../../lib/cn";
import { graphHandleToneFromId } from "../lib/activity-graph-handle-tone";

export type PlaceNodeType =
  | "constraint"
  | "doc"
  | "resource"
  | "statePosition"
  | "worker"
  | "workType";

export type ActivityGraphNodeHandle = GraphNodeHandle;

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
  const packageHandles = handles.map((handle) => ({
    ...handle,
    tone: handle.tone ?? graphHandleToneFromId(handle.id),
  }));
  const activeZAxisIncompleteHints =
    nodeType === "workstation" ? zAxisIncompleteHints : null;
  const zAxisHintSlots = activeZAxisIncompleteHints
    ? workstationZAxisIncompleteHintSlots()
    : [];

  return (
    <div className="relative h-full min-w-0 w-full">
      <GraphNodeShell
        className={className}
        data-current-activity-node-type={nodeType}
        handles={packageHandles}
        nodeKind={nodeType}
        showStateIndicator={false}
      >
        {children}
      </GraphNodeShell>
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
        className={cn(
          "block h-2.5 w-2.5 rounded-full border border-af-danger-border bg-error shadow-[0_0_0_3px_var(--color-error-container)] motion-safe:animate-pulse",
        )}
      />
    </span>
  );
}
