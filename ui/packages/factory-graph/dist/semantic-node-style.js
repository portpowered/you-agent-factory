const SURFACE_TONE_CLASS_NAME = {
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
const NODE_TITLE_CLASS_NAME = "block min-w-0 max-w-full whitespace-normal break-words [overflow-wrap:anywhere] font-bold leading-tight text-on-surface";
const NODE_WRAPPED_TEXT_CLASS_NAME = "min-w-0 max-w-full whitespace-normal break-words [overflow-wrap:anywhere]";
const VISUAL_STATUS_SURFACE_CLASS_NAME = {
    quiet: "",
    waiting: "border-info-border bg-info-container",
    active: "border-af-success-border bg-warning-container",
    success: "border-af-success-border bg-success-container",
    danger: "border-af-danger-border bg-error-container",
};
const VISUAL_STATUS_IMPORTANT_SURFACE_CLASS_NAME = {
    quiet: "",
    waiting: "!bg-info-container",
    active: "!bg-warning-container",
    success: "!bg-success-container",
    danger: "!bg-error-container",
};
const VISUAL_STATUS_BORDER_CLASS_NAME = {
    quiet: "",
    waiting: "border-info-border",
    active: "border-af-success-border",
    success: "border-af-success-border",
    danger: "border-af-danger-border",
};
const VISUAL_STATUS_ICON_CLASS_NAME = {
    quiet: "text-on-surface-variant",
    waiting: "text-info",
    active: "text-warning",
    success: "text-success",
    danger: "text-error",
};
const HOVER_CLASS_BY_SURFACE = {
    primary: "transition-[background-color,border-color,box-shadow,opacity] hover:border-primary hover:bg-primary-container hover:opacity-100 hover:shadow-af-accent-chip",
    warning: "transition-[background-color,border-color,box-shadow,opacity] hover:border-primary hover:bg-warning-container hover:opacity-100 hover:shadow-af-accent-chip",
};
export function factoryGraphNodeSurfaceClassName(tone) {
    return SURFACE_TONE_CLASS_NAME[tone];
}
export function factoryGraphNodeTitleClassName(className) {
    return [NODE_TITLE_CLASS_NAME, className].filter(Boolean).join(" ");
}
/** Safe wrapping shared by semantic labels and their merged metadata surfaces. */
export function factoryGraphNodeWrappedTextClassName(className) {
    return [NODE_WRAPPED_TEXT_CLASS_NAME, className].filter(Boolean).join(" ");
}
/** Surface and emphasis classes for the package-owned visual-state grammar. */
export function factoryGraphNodeVisualStateClassName(state) {
    const validationBorderClassName = state.validation === "warning"
        ? "!border-warning"
        : state.validation === "error"
            ? "!border-error"
            : undefined;
    const borderClassName = state.border === "selection"
        ? "!border-primary shadow-af-accent-selected"
        : state.border === "validation"
            ? validationBorderClassName
            : VISUAL_STATUS_BORDER_CLASS_NAME[state.border];
    const validationGlowClassName = state.validation === "warning"
        ? "shadow-af-graph-validation-warning motion-safe:animate-pulse"
        : state.validation === "error"
            ? "shadow-af-graph-validation-danger motion-safe:animate-pulse"
            : undefined;
    const glowClassName = state.glow === "active"
        ? "shadow-af-success-chip"
        : state.glow === "danger"
            ? "shadow-af-graph-danger"
            : state.glow === "selection"
                ? "shadow-af-accent-selected"
                : state.glow === "validation"
                    ? validationGlowClassName
                    : undefined;
    const focusClassName = state.focus === "keyboard" || state.focus === "selection-and-keyboard"
        ? "ring-2 ring-af-graph-focus-indicator"
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
        state.muted && "agent-flow-node--muted",
    ]
        .filter(Boolean)
        .join(" ");
}
/** Returns a lifecycle/active icon class, with a family fallback for idle nodes. */
export function factoryGraphNodeVisualIconClassName(state, fallbackClassName = "text-on-surface-variant") {
    return state.icon === "quiet"
        ? fallbackClassName
        : VISUAL_STATUS_ICON_CLASS_NAME[state.icon];
}
/** Plain surface classes used by phase legends and compatibility adapters. */
export function factoryGraphNodeVisualStatusSurfaceClassName(status) {
    return VISUAL_STATUS_SURFACE_CLASS_NAME[status];
}
/** Accent feedback used by the original Factory graph's semantic node views. */
export function factoryGraphNodeHoverClassName(state, surface = "warning") {
    if (state.selected || state.validationError)
        return undefined;
    return HOVER_CLASS_BY_SURFACE[surface];
}
