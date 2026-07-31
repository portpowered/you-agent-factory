import {
  type GraphNodeHandle,
  GraphNodeShell,
} from "@you-agent-factory/components/graphs";
import type { ReactNode } from "react";

export type FactoryGraphPlaceNodeType =
  | "constraint"
  | "doc"
  | "resource"
  | "statePosition"
  | "worker"
  | "workType";

export type FactoryGraphNodeHandle = GraphNodeHandle;

export interface FactoryGraphZAxisIncompleteHints {
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

export interface FactoryGraphNodeShellProps {
  children: ReactNode;
  className?: string;
  handles: FactoryGraphNodeHandle[];
  nodeType: "workstation" | FactoryGraphPlaceNodeType;
  zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
}

/** Original semantic Factory node frame, including its typed connection rails. */
export function FactoryGraphNodeShell({
  children,
  className = "",
  handles,
  nodeType,
  zAxisIncompleteHints = null,
}: FactoryGraphNodeShellProps) {
  const packageHandles = handles.map((handle) => ({
    ...handle,
    tone: handle.tone ?? factoryGraphHandleToneFromId(handle.id),
  }));
  const activeHints = nodeType === "workstation" ? zAxisIncompleteHints : null;

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
      {activeHints
        ? workstationZAxisIncompleteHintSlots().map((slot) => (
            <ZAxisIncompleteHintOrb
              accessibleLabel={activeHints.accessibleLabel}
              anchorId={slot.anchorId}
              key={slot.anchorId}
              title={activeHints.title}
              top={slot.top}
            />
          ))
        : null}
    </div>
  );
}

export function factoryGraphHandleToneFromId(
  handleId: string,
): NonNullable<GraphNodeHandle["tone"]> {
  if (handleId.includes("resource")) return "resource";
  if (
    handleId.includes("worker-assignment") ||
    handleId.includes("worker-input")
  )
    return "worker";
  if (handleId.includes("on-continue")) return "continue";
  if (handleId.includes("on-failure")) return "failure";
  if (handleId.includes("on-rejection")) return "rejection";
  if (handleId.includes("output")) return "output";
  if (handleId.includes("input")) return "input";
  if (handleId.includes("assignment")) return "assignment";
  return "default";
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
      top: `${((index + 1) * 100) / (railCount + 1)}%`,
    };
  });
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
