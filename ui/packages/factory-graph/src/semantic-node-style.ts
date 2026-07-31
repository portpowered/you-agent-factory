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

/** Accent feedback used by the original Factory graph's semantic node views. */
export function factoryGraphNodeHoverClassName(
  state: FactoryGraphNodeHoverState,
  surface: FactoryGraphNodeHoverSurface = "warning",
): string | undefined {
  if (state.selected || state.validationError) return undefined;
  return HOVER_CLASS_BY_SURFACE[surface];
}
