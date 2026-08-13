import type { FactoryGraphVisualState } from "./visual-state.js";

export type FactoryGraphNodeSurfaceTone =
  | "danger"
  | "info"
  | "neutral"
  | "neutralHigh"
  | "primary"
  | "resource"
  | "success"
  | "warning"
  | "workState"
  | "workstation";

const SURFACE_TONE_CLASS_NAME: Record<FactoryGraphNodeSurfaceTone, string> = {
  danger: "border-af-danger-border bg-error-container",
  info: "border-info-border bg-info-container",
  neutral: "border-outline bg-surface",
  neutralHigh: "border-outline-variant bg-surface-container-high",
  primary: "border-primary bg-primary-container",
  resource: "border-outline bg-background",
  success: "border-af-success-border bg-success-container",
  warning: "border-af-warning-border bg-warning-container",
  workState: "border-info-border bg-info-container",
  workstation: "border-outline-variant bg-surface-container-highest",
};

const NODE_TITLE_CLASS_NAME =
  "block min-w-0 truncate whitespace-nowrap font-bold leading-tight text-on-surface";

const VISUAL_STATUS_SURFACE_CLASS_NAME: Record<
  FactoryGraphVisualState["surface"],
  string
> = {
  quiet: "",
  waiting: "border-info-border bg-info-container",
  active: "border-af-success-border bg-warning-container",
  success: "border-af-success-border bg-success-container",
  danger: "border-af-danger-border bg-error-container",
};

const VISUAL_STATUS_IMPORTANT_SURFACE_CLASS_NAME: Record<
  FactoryGraphVisualState["surface"],
  string
> = {
  quiet: "",
  waiting: "!border-info-border !bg-info-container",
  active: "!border-af-success-border !bg-warning-container",
  success: "!border-af-success-border !bg-success-container",
  danger: "!border-af-danger-border !bg-error-container",
};

const VISUAL_STATUS_ICON_CLASS_NAME: Record<
  FactoryGraphVisualState["icon"],
  string
> = {
  quiet: "text-on-surface-variant",
  waiting: "text-info",
  active: "text-warning",
  success: "text-success",
  danger: "text-error",
};

export interface FactoryGraphNodeHoverState {
  activeFlow?: boolean;
  muted?: boolean;
  selected?: boolean;
  validationError?: boolean;
}

export type FactoryGraphNodeHoverSurface = "primary" | "warning";

const HOVER_CLASS_BY_SURFACE: Record<FactoryGraphNodeHoverSurface, string> = {
  primary:
    "transition-[background-color,border-color,box-shadow,opacity] hover:border-primary hover:bg-primary-container hover:opacity-100 hover:shadow-af-accent-chip",
  warning:
    "transition-[background-color,border-color,box-shadow,opacity] hover:border-primary hover:bg-warning-container hover:opacity-100 hover:shadow-af-accent-chip",
};

export function factoryGraphNodeSurfaceClassName(
  tone: FactoryGraphNodeSurfaceTone,
): string {
  return SURFACE_TONE_CLASS_NAME[tone];
}

export function factoryGraphNodeTitleClassName(className?: string): string {
  return [NODE_TITLE_CLASS_NAME, className].filter(Boolean).join(" ");
}

/** Surface and emphasis classes for the package-owned visual-state grammar. */
export function factoryGraphNodeVisualStateClassName(
  state: FactoryGraphVisualState,
): string {
  const validationClassName =
    state.validation === "warning"
      ? "border-af-warning-border ring-2 ring-af-warning-border motion-safe:animate-pulse"
      : state.validation === "error"
        ? "border-af-danger-border ring-2 ring-af-danger-border motion-safe:animate-pulse"
        : undefined;
  const borderClassName =
    state.border === "selection"
      ? "border-primary shadow-af-accent-selected"
      : state.border === "validation"
        ? validationClassName
        : undefined;
  const glowClassName =
    state.glow === "active"
      ? "shadow-af-success-chip"
      : state.glow === "danger"
        ? "shadow-af-graph-danger"
        : state.glow === "selection"
          ? "shadow-af-accent-selected"
          : state.glow === "validation"
            ? validationClassName
            : undefined;
  const focusClassName =
    state.focus === "keyboard" || state.focus === "selection-and-keyboard"
      ? "ring-2 ring-af-focus-ring"
      : undefined;

  return [
    VISUAL_STATUS_IMPORTANT_SURFACE_CLASS_NAME[state.surface],
    borderClassName,
    glowClassName,
    focusClassName,
    state.activeFlow &&
      state.glow === "active" &&
      "agent-flow-node--active ring-2 ring-af-success-border",
    state.selection && "shadow-af-accent-selected",
    state.muted && "opacity-[0.45]",
  ]
    .filter(Boolean)
    .join(" ");
}

/** Returns a lifecycle/active icon class, with a family fallback for idle nodes. */
export function factoryGraphNodeVisualIconClassName(
  state: FactoryGraphVisualState,
  fallbackClassName = "text-on-surface-variant",
): string {
  return state.icon === "quiet"
    ? fallbackClassName
    : VISUAL_STATUS_ICON_CLASS_NAME[state.icon];
}

/** Plain surface classes used by phase legends and compatibility adapters. */
export function factoryGraphNodeVisualStatusSurfaceClassName(
  status: FactoryGraphVisualState["surface"],
): string {
  return VISUAL_STATUS_SURFACE_CLASS_NAME[status];
}

/** Accent feedback used by the original Factory graph's semantic node views. */
export function factoryGraphNodeHoverClassName(
  state: FactoryGraphNodeHoverState,
  surface: FactoryGraphNodeHoverSurface = "warning",
): string | undefined {
  if (state.selected || state.validationError) return undefined;
  return HOVER_CLASS_BY_SURFACE[surface];
}
