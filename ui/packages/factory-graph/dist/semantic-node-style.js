import { factoryGraphVisualNestedAccentRole } from "./visual-state.js";
const NESTED_ACCENT_SURFACE_CLASS_NAME = {
    neutral: "",
    info: "border-info-border bg-info-container",
    warning: "border-af-warning-border bg-warning-container",
    success: "border-af-success-border bg-success-container",
    danger: "border-af-danger-border bg-error-container",
};
const NESTED_ACCENT_IMPORTANT_SURFACE_CLASS_NAME = {
    neutral: "",
    info: "!bg-info-container",
    warning: "!bg-warning",
    success: "!bg-success-container",
    danger: "!bg-error-container",
};
const NESTED_ACCENT_ACTIVE_TEXT_OVERRIDE_CLASS_NAME = "[&_[data-factory-entity-title]]:!text-on-warning [&_[data-workstation-runtime-label]]:!text-on-warning [&_[data-active-work-label]]:!text-on-warning [&_[data-active-work-duration]]:!text-on-warning [&_[data-workstation-scheduling-label]]:!border-on-warning [&_[data-workstation-scheduling-label]]:!text-on-warning";
const NESTED_ACCENT_BORDER_CLASS_NAME = {
    neutral: "",
    info: "border-info-border",
    warning: "border-af-warning-border",
    success: "border-af-success-border",
    danger: "border-af-danger-border",
};
const NESTED_ACCENT_ICON_CLASS_NAME = {
    neutral: "text-on-surface-variant",
    info: "text-info",
    warning: "text-warning",
    success: "text-success",
    danger: "text-error",
};
const NESTED_ACCENT_GLOW_CLASS_NAME = {
    neutral: "shadow-none",
    info: "shadow-af-info-active",
    warning: "shadow-af-graph-warning",
    success: "shadow-af-success-chip",
    danger: "shadow-af-graph-danger",
};
const NESTED_ACCENT_RING_CLASS_NAME = {
    neutral: "ring-outline",
    info: "ring-info-border",
    warning: "ring-af-warning-border",
    success: "ring-af-success-border",
    danger: "ring-af-danger-border",
};
const NESTED_ACCENT_GUARDED_CARD_CLASS_NAME = {
    neutral: "[&_[data-workstation-guard-card]]:!border-outline [&_[data-workstation-guard-card]]:!bg-surface [&_[data-workstation-guard-card]]:!text-on-surface",
    info: "[&_[data-workstation-guard-card]]:!border-info-border [&_[data-workstation-guard-card]]:!bg-info-container [&_[data-workstation-guard-card]]:!text-on-info-container",
    warning: "[&_[data-workstation-guard-card]]:!border-af-warning-border [&_[data-workstation-guard-card]]:!bg-warning-container [&_[data-workstation-guard-card]]:!text-on-warning-container",
    success: "[&_[data-workstation-guard-card]]:!border-af-success-border [&_[data-workstation-guard-card]]:!bg-success-container [&_[data-workstation-guard-card]]:!text-on-success-container",
    danger: "[&_[data-workstation-guard-card]]:!border-af-danger-border [&_[data-workstation-guard-card]]:!bg-error-container [&_[data-workstation-guard-card]]:!text-on-error-container",
};
function nestedAccentClassNameForStatus(status, classNames) {
    return classNames[factoryGraphVisualNestedAccentRole(status)];
}
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
    quiet: nestedAccentClassNameForStatus("quiet", NESTED_ACCENT_SURFACE_CLASS_NAME),
    waiting: nestedAccentClassNameForStatus("waiting", NESTED_ACCENT_SURFACE_CLASS_NAME),
    active: nestedAccentClassNameForStatus("active", NESTED_ACCENT_SURFACE_CLASS_NAME),
    success: nestedAccentClassNameForStatus("success", NESTED_ACCENT_SURFACE_CLASS_NAME),
    danger: nestedAccentClassNameForStatus("danger", NESTED_ACCENT_SURFACE_CLASS_NAME),
};
const VISUAL_STATUS_IMPORTANT_SURFACE_CLASS_NAME = {
    quiet: nestedAccentClassNameForStatus("quiet", NESTED_ACCENT_IMPORTANT_SURFACE_CLASS_NAME),
    waiting: nestedAccentClassNameForStatus("waiting", NESTED_ACCENT_IMPORTANT_SURFACE_CLASS_NAME),
    active: nestedAccentClassNameForStatus("active", NESTED_ACCENT_IMPORTANT_SURFACE_CLASS_NAME),
    success: nestedAccentClassNameForStatus("success", NESTED_ACCENT_IMPORTANT_SURFACE_CLASS_NAME),
    danger: nestedAccentClassNameForStatus("danger", NESTED_ACCENT_IMPORTANT_SURFACE_CLASS_NAME),
};
const VISUAL_STATUS_BORDER_CLASS_NAME = {
    quiet: nestedAccentClassNameForStatus("quiet", NESTED_ACCENT_BORDER_CLASS_NAME),
    waiting: nestedAccentClassNameForStatus("waiting", NESTED_ACCENT_BORDER_CLASS_NAME),
    active: nestedAccentClassNameForStatus("active", NESTED_ACCENT_BORDER_CLASS_NAME),
    success: nestedAccentClassNameForStatus("success", NESTED_ACCENT_BORDER_CLASS_NAME),
    danger: nestedAccentClassNameForStatus("danger", NESTED_ACCENT_BORDER_CLASS_NAME),
};
const VISUAL_STATUS_ICON_CLASS_NAME = {
    quiet: nestedAccentClassNameForStatus("quiet", NESTED_ACCENT_ICON_CLASS_NAME),
    waiting: nestedAccentClassNameForStatus("waiting", NESTED_ACCENT_ICON_CLASS_NAME),
    active: nestedAccentClassNameForStatus("active", NESTED_ACCENT_ICON_CLASS_NAME),
    success: nestedAccentClassNameForStatus("success", NESTED_ACCENT_ICON_CLASS_NAME),
    danger: nestedAccentClassNameForStatus("danger", NESTED_ACCENT_ICON_CLASS_NAME),
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
    const nestedAccentRole = factoryGraphVisualNestedAccentRole(state.surface);
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
        ? NESTED_ACCENT_GLOW_CLASS_NAME[nestedAccentRole]
        : state.glow === "danger"
            ? NESTED_ACCENT_GLOW_CLASS_NAME.danger
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
        state.surface === "active" && NESTED_ACCENT_ACTIVE_TEXT_OVERRIDE_CLASS_NAME,
        borderClassName,
        glowClassName,
        focusClassName,
        state.glow === "active" &&
            `agent-flow-node--active ring-2 ${NESTED_ACCENT_RING_CLASS_NAME[nestedAccentRole]}`,
        state.selection && "shadow-af-accent-selected",
        state.muted && "agent-flow-node--muted",
    ]
        .filter(Boolean)
        .join(" ");
}
/** Applies the owning node tone to nested guarded workstation content. */
export function factoryGraphNodeVisualNestedAccentClassName(state) {
    return NESTED_ACCENT_GUARDED_CARD_CLASS_NAME[factoryGraphVisualNestedAccentRole(state.surface)];
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
