const NESTED_ACCENT_ROLE_BY_STATUS = {
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
export function factoryGraphVisualNestedAccentRole(parentStatus) {
    return NESTED_ACCENT_ROLE_BY_STATUS[parentStatus];
}
/**
 * Resolve one composable visual grammar for a semantic Factory node.
 *
 * Precedence is explicit: lifecycle status is always retained in `status`;
 * active flow may elevate a quiet node's `surface`, `icon`, and
 * `statusTreatment`; validation overlays selection and active-flow emphasis;
 * selection/focus overlays active-flow emphasis; and muted is returned as an
 * independent flag so it cannot erase any status.
 *
 * Tone and occupancy are separate axes. `surface` carries the authored tone
 * even when the node is empty; only held Work promotes `fill` to `solid` and
 * lights the active glow, so an authored `PROCESSING` work state stays a
 * translucent container until Work actually enters it.
 */
export function resolveFactoryGraphVisualState(input) {
    const lifecycle = resolveLifecycleRole(input.lifecycle, input.runtimeStatus);
    const status = statusRoleForLifecycle(lifecycle);
    const validation = normalizeValidation(input.validation);
    const selected = input.selected === true;
    const focused = input.focused === true;
    const activeFlow = input.activeFlow === true;
    const muted = input.muted === true;
    const hasSelectionOrFocus = selected || focused;
    const activeFlowStatus = status === "quiet" && activeFlow ? "active" : status;
    const holdsWork = activeFlow || input.activeWork === true;
    let border = status;
    if (validation !== "none") {
        border = "validation";
    }
    else if (hasSelectionOrFocus) {
        border = "selection";
    }
    else if (status === "quiet" && activeFlow) {
        border = "active";
    }
    let glow = "none";
    if (validation !== "none") {
        glow = "validation";
    }
    else if (hasSelectionOrFocus) {
        glow = "selection";
    }
    else if (holdsWork) {
        glow = "active";
    }
    else if (lifecycle === "failed") {
        glow = "danger";
    }
    return {
        activeFlow,
        border,
        emphasis: emphasisFor({
            hasSelectionOrFocus,
            holdsWork,
            status,
            validation,
        }),
        family: input.family,
        fill: holdsWork && activeFlowStatus !== "quiet" ? "solid" : "soft",
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
function resolveLifecycleRole(lifecycle, runtimeStatus) {
    return (lifecycleRoleFromValue(lifecycle) ??
        lifecycleRoleFromValue(runtimeStatus) ??
        "unknown");
}
function lifecycleRoleFromValue(value) {
    if (!value || value.trim().length === 0)
        return undefined;
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
function normalizeStatusValue(value) {
    return value
        .trim()
        .toUpperCase()
        .replace(/[\s-]+/g, "_");
}
function statusRoleForLifecycle(lifecycle) {
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
function statusTreatmentFor(lifecycle, activeFlow) {
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
function normalizeValidation(validation) {
    if (validation === true)
        return "error";
    if (validation === false || validation === null || validation === undefined) {
        return "none";
    }
    return validation;
}
function focusRoleFor(selected, focused) {
    if (selected && focused)
        return "selection-and-keyboard";
    if (selected)
        return "selection";
    if (focused)
        return "keyboard";
    return "none";
}
function emphasisFor(input) {
    if (input.validation !== "none")
        return "attention";
    if (input.hasSelectionOrFocus)
        return "selected";
    if (input.holdsWork)
        return "strong";
    return input.status === "quiet" ? "quiet" : "standard";
}
