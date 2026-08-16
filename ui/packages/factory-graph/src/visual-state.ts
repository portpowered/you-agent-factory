import type { FactoryGraphNodeFamily } from "./node-family.js";

export type FactoryGraphVisualLifecycleRole =
  | "unknown"
  | "initial"
  | "processing"
  | "terminal"
  | "failed";

export type FactoryGraphVisualStatusRole =
  | "quiet"
  | "waiting"
  | "active"
  | "success"
  | "danger";

/** Semantic role used by nested accents owned by a graph node. */
export type FactoryGraphVisualNestedAccentRole =
  | "neutral"
  | "info"
  | "warning"
  | "success"
  | "danger";

const NESTED_ACCENT_ROLE_BY_STATUS: Record<
  FactoryGraphVisualStatusRole,
  FactoryGraphVisualNestedAccentRole
> = {
  quiet: "neutral",
  waiting: "info",
  active: "warning",
  success: "success",
  danger: "danger",
};

/**
 * Derive the semantic role for a nested accent from its parent's resolved
 * visual status. Callers can use the result for surfaces, borders, readable
 * foregrounds, glows, rings, badges, and icons without copying this policy.
 */
export function factoryGraphVisualNestedAccentRole(
  parentStatus: FactoryGraphVisualStatusRole,
): FactoryGraphVisualNestedAccentRole {
  return NESTED_ACCENT_ROLE_BY_STATUS[parentStatus];
}

export type FactoryGraphVisualBorderRole =
  | FactoryGraphVisualStatusRole
  | "selection"
  | "validation";

export type FactoryGraphVisualGlowRole =
  | "none"
  | "active"
  | "danger"
  | "selection"
  | "validation";

export type FactoryGraphVisualStatusTreatment =
  | "none"
  | "waiting"
  | "processing"
  | "completed"
  | "failed";

export type FactoryGraphVisualEmphasis =
  | "quiet"
  | "standard"
  | "strong"
  | "attention"
  | "selected";

export type FactoryGraphVisualFocusRole =
  | "none"
  | "keyboard"
  | "selection"
  | "selection-and-keyboard";

export type FactoryGraphValidationState = "none" | "warning" | "error";

export interface FactoryGraphVisualStateInput {
  /** Protocol-neutral Factory graph family, not a transport node type. */
  family: FactoryGraphNodeFamily;
  /** Work-state lifecycle or an equivalent host runtime status. */
  lifecycle?: string | null;
  runtimeStatus?: string | null;
  selected?: boolean;
  focused?: boolean;
  validation?: FactoryGraphValidationState | boolean | null;
  activeFlow?: boolean;
  muted?: boolean;
}

export interface FactoryGraphVisualState {
  family: FactoryGraphNodeFamily;
  lifecycle: FactoryGraphVisualLifecycleRole;
  status: FactoryGraphVisualStatusRole;
  surface: FactoryGraphVisualStatusRole;
  border: FactoryGraphVisualBorderRole;
  glow: FactoryGraphVisualGlowRole;
  icon: FactoryGraphVisualStatusRole;
  statusTreatment: FactoryGraphVisualStatusTreatment;
  emphasis: FactoryGraphVisualEmphasis;
  focus: FactoryGraphVisualFocusRole;
  selection: boolean;
  validation: FactoryGraphValidationState;
  activeFlow: boolean;
  muted: boolean;
}

/**
 * Resolve one composable visual grammar for a semantic Factory node.
 *
 * Precedence is explicit: lifecycle status is always retained in `status`;
 * active flow may elevate a quiet node's `surface`, `icon`, and
 * `statusTreatment`; validation overlays selection and active-flow emphasis;
 * selection/focus overlays active-flow emphasis; and muted is returned as an
 * independent flag so it cannot erase any status.
 */
export function resolveFactoryGraphVisualState(
  input: FactoryGraphVisualStateInput,
): FactoryGraphVisualState {
  const lifecycle = resolveLifecycleRole(input.lifecycle, input.runtimeStatus);
  const status = statusRoleForLifecycle(lifecycle);
  const validation = normalizeValidation(input.validation);
  const selected = input.selected === true;
  const focused = input.focused === true;
  const activeFlow = input.activeFlow === true;
  const muted = input.muted === true;
  const hasSelectionOrFocus = selected || focused;
  const activeFlowStatus = status === "quiet" && activeFlow ? "active" : status;

  let border: FactoryGraphVisualBorderRole = status;
  if (validation !== "none") {
    border = "validation";
  } else if (hasSelectionOrFocus) {
    border = "selection";
  } else if (status === "quiet" && activeFlow) {
    border = "active";
  }

  let glow: FactoryGraphVisualGlowRole = "none";
  if (validation !== "none") {
    glow = "validation";
  } else if (hasSelectionOrFocus) {
    glow = "selection";
  } else if (activeFlow || lifecycle === "processing") {
    glow = "active";
  } else if (lifecycle === "failed") {
    glow = "danger";
  }

  return {
    activeFlow,
    border,
    emphasis: emphasisFor({
      activeFlow,
      hasSelectionOrFocus,
      lifecycle,
      status,
      validation,
    }),
    family: input.family,
    focus: focusRoleFor(selected, focused),
    glow,
    icon: activeFlowStatus,
    lifecycle,
    muted,
    selection: selected,
    status,
    statusTreatment: statusTreatmentFor(lifecycle, activeFlow),
    surface: activeFlowStatus,
    validation,
  };
}

function resolveLifecycleRole(
  lifecycle: string | null | undefined,
  runtimeStatus: string | null | undefined,
): FactoryGraphVisualLifecycleRole {
  return (
    lifecycleRoleFromValue(lifecycle) ??
    lifecycleRoleFromValue(runtimeStatus) ??
    "unknown"
  );
}

function lifecycleRoleFromValue(
  value: string | null | undefined,
): FactoryGraphVisualLifecycleRole | undefined {
  if (!value || value.trim().length === 0) return undefined;

  switch (normalizeStatusValue(value)) {
    case "INITIAL":
    case "QUEUED":
    case "PENDING":
    case "WAITING":
    case "READY":
      return "initial";
    case "PROCESSING":
    case "ACTIVE":
    case "RUNNING":
    case "STARTING":
    case "IN_PROGRESS":
      return "processing";
    case "TERMINAL":
    case "ACCEPTED":
    case "COMPLETED":
    case "COMPLETE":
    case "CONTINUE":
    case "SUCCESS":
    case "SUCCEEDED":
    case "DONE":
      return "terminal";
    case "FAILED":
    case "FAILURE":
    case "ERROR":
    case "REJECTED":
    case "REJECT":
    case "CANCELED":
    case "CANCELLED":
      return "failed";
    default:
      return undefined;
  }
}

function normalizeStatusValue(value: string): string {
  return value
    .trim()
    .toUpperCase()
    .replace(/[\s-]+/g, "_");
}

function statusRoleForLifecycle(
  lifecycle: FactoryGraphVisualLifecycleRole,
): FactoryGraphVisualStatusRole {
  switch (lifecycle) {
    case "initial":
      return "waiting";
    case "processing":
      return "active";
    case "terminal":
      return "success";
    case "failed":
      return "danger";
    case "unknown":
      return "quiet";
  }
}

function statusTreatmentFor(
  lifecycle: FactoryGraphVisualLifecycleRole,
  activeFlow: boolean,
): FactoryGraphVisualStatusTreatment {
  switch (lifecycle) {
    case "initial":
      return "waiting";
    case "processing":
      return "processing";
    case "terminal":
      return "completed";
    case "failed":
      return "failed";
    case "unknown":
      return activeFlow ? "processing" : "none";
  }
}

function normalizeValidation(
  validation: FactoryGraphVisualStateInput["validation"],
): FactoryGraphValidationState {
  if (validation === true) return "error";
  if (validation === false || validation === null || validation === undefined) {
    return "none";
  }
  return validation;
}

function focusRoleFor(
  selected: boolean,
  focused: boolean,
): FactoryGraphVisualFocusRole {
  if (selected && focused) return "selection-and-keyboard";
  if (selected) return "selection";
  if (focused) return "keyboard";
  return "none";
}

function emphasisFor(input: {
  activeFlow: boolean;
  hasSelectionOrFocus: boolean;
  lifecycle: FactoryGraphVisualLifecycleRole;
  status: FactoryGraphVisualStatusRole;
  validation: FactoryGraphValidationState;
}): FactoryGraphVisualEmphasis {
  if (input.validation !== "none") return "attention";
  if (input.hasSelectionOrFocus) return "selected";
  if (input.activeFlow || input.lifecycle === "processing") return "strong";
  return input.status === "quiet" ? "quiet" : "standard";
}
