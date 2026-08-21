import {
  type GraphNodeHandle,
  GraphNodeShell,
} from "@you-agent-factory/components/graphs";
import type { ReactNode } from "react";

import {
  factoryGraphNodeFamilyForShellType,
  factoryGraphNodeFamilyRole,
} from "./node-family.js";
import {
  type FactoryGraphNodeInteractionOverlay,
  FactoryGraphNodeInteractionOverlayView,
} from "./node-interaction-overlay.js";
import {
  FactoryGraphNodeResizeControls,
  type FactoryGraphNodeResizeControlsProps,
} from "./node-resize-controls.js";
import {
  factoryGraphNodeVisualNestedAccentClassName,
  factoryGraphNodeVisualStateClassName,
} from "./semantic-node-style.js";
import {
  factoryGraphVisualNestedAccentRole,
  type FactoryGraphVisualStateInput,
  resolveFactoryGraphVisualState,
} from "./visual-state.js";

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
  interactionOverlay?: FactoryGraphNodeInteractionOverlay;
  nodeType: "workstation" | FactoryGraphPlaceNodeType;
  resizeControls?: FactoryGraphNodeResizeControlsProps;
  visualState?: Omit<FactoryGraphVisualStateInput, "family">;
  zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
}

/** Original semantic Factory node frame, including its typed connection rails. */
export function FactoryGraphNodeShell({
  children,
  className = "",
  handles,
  interactionOverlay,
  nodeType,
  resizeControls,
  visualState: visualStateInput,
  zAxisIncompleteHints = null,
}: FactoryGraphNodeShellProps) {
  const packageHandles = handles.map((handle) => ({
    ...handle,
    tone: handle.tone ?? factoryGraphHandleToneFromId(handle.id),
  }));
  const activeHints = nodeType === "workstation" ? zAxisIncompleteHints : null;
  const familyRole = factoryGraphNodeFamilyRole(
    factoryGraphNodeFamilyForShellType(nodeType),
  );
  const visualState = resolveFactoryGraphVisualState({
    family: familyRole.family,
    ...visualStateInput,
  });
  const nestedAccentRole = factoryGraphVisualNestedAccentRole(
    visualState.surface,
  );
  const visibleResizeControls = resizeControls
    ? { ...resizeControls, isVisible: visualState.selection }
    : undefined;

  return (
    // `group/factory-graph-node` is the hover/focus ancestor the resize grip
    // reveals against; keep it on the wrapper that spans the whole card.
    <div className="group/factory-graph-node relative h-full min-w-0 w-full">
      <GraphNodeShell
        aria-invalid={visualState.validation === "error" || undefined}
        className={classNames(
          "shadow-none",
          nodeType === "workstation" &&
            "[&>div:last-of-type]:gap-0.5 [&>div:last-of-type]:py-2",
          className,
          interactionOverlayClassName(interactionOverlay?.draftStatus),
          factoryGraphNodeVisualStateClassName(visualState),
          factoryGraphNodeVisualNestedAccentClassName(visualState),
        )}
        data-current-activity-node-type={nodeType}
        data-graph-node-family={familyRole.family}
        data-graph-node-shape={familyRole.shape}
        data-graph-visual-active-flow={visualState.activeFlow || undefined}
        data-graph-visual-border={visualState.border}
        data-graph-visual-emphasis={visualState.emphasis}
        data-graph-visual-fill={visualState.fill}
        data-graph-visual-focus={visualState.focus}
        data-graph-visual-glow={visualState.glow}
        data-graph-visual-icon={visualState.icon}
        data-graph-visual-lifecycle={visualState.lifecycle}
        data-graph-visual-muted={visualState.muted || undefined}
        data-graph-visual-nested-accent={nestedAccentRole}
        data-graph-visual-selection={visualState.selection || undefined}
        data-graph-visual-status={visualState.status}
        data-graph-visual-surface={visualState.surface}
        data-graph-visual-treatment={visualState.statusTreatment}
        data-graph-visual-validation={visualState.validation}
        data-graph-draft-status={
          interactionOverlay?.draftStatus === "none"
            ? undefined
            : interactionOverlay?.draftStatus
        }
        handles={packageHandles}
        nodeKind={nodeType}
        showStateIndicator={false}
      >
        {children}
        <FactoryGraphNodeInteractionOverlayView overlay={interactionOverlay} />
      </GraphNodeShell>
      {visibleResizeControls ? (
        <FactoryGraphNodeResizeControls {...visibleResizeControls} />
      ) : null}
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

function classNames(
  ...values: Array<string | false | null | undefined>
): string {
  return values.filter(Boolean).join(" ");
}

function interactionOverlayClassName(
  draftStatus: FactoryGraphNodeInteractionOverlay["draftStatus"],
): string | undefined {
  switch (draftStatus) {
    case "addition":
      return "ring-2 ring-af-warning-border";
    case "removal":
      return "ring-2 ring-af-danger-border";
    default:
      return undefined;
  }
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
  if (handleId.includes("approval")) return "output";
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
