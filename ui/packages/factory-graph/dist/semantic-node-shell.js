import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { GraphNodeShell, } from "@you-agent-factory/components/graphs";
import { factoryGraphNodeFamilyForShellType, factoryGraphNodeFamilyRole, } from "./node-family.js";
import { FactoryGraphNodeResizeControls, } from "./node-resize-controls.js";
import { factoryGraphNodeVisualStateClassName } from "./semantic-node-style.js";
import { resolveFactoryGraphVisualState, } from "./visual-state.js";
const WORKSTATION_RIGHT_RAIL_ANCHOR_IDS = [
    "workstation-output-source",
    "workstation-on-continue-source",
    "workstation-on-failure-source",
    "workstation-on-rejection-source",
];
const Z_AXIS_INCOMPLETE_HINT_ANCHOR_IDS = [
    "workstation-on-continue-source",
    "workstation-on-rejection-source",
];
/** Original semantic Factory node frame, including its typed connection rails. */
export function FactoryGraphNodeShell({ children, className = "", handles, nodeType, resizeControls, visualState: visualStateInput, zAxisIncompleteHints = null, }) {
    const packageHandles = handles.map((handle) => ({
        ...handle,
        tone: handle.tone ?? factoryGraphHandleToneFromId(handle.id),
    }));
    const activeHints = nodeType === "workstation" ? zAxisIncompleteHints : null;
    const familyRole = factoryGraphNodeFamilyRole(factoryGraphNodeFamilyForShellType(nodeType));
    const visualState = resolveFactoryGraphVisualState({
        family: familyRole.family,
        ...visualStateInput,
    });
    return (_jsxs("div", { className: "relative h-full min-w-0 w-full", children: [_jsx(GraphNodeShell, { "aria-invalid": visualState.validation === "error" || undefined, className: classNames(className, factoryGraphNodeVisualStateClassName(visualState)), "data-current-activity-node-type": nodeType, "data-graph-node-family": familyRole.family, "data-graph-node-shape": familyRole.shape, "data-graph-visual-active-flow": visualState.activeFlow || undefined, "data-graph-visual-border": visualState.border, "data-graph-visual-emphasis": visualState.emphasis, "data-graph-visual-focus": visualState.focus, "data-graph-visual-glow": visualState.glow, "data-graph-visual-icon": visualState.icon, "data-graph-visual-lifecycle": visualState.lifecycle, "data-graph-visual-muted": visualState.muted || undefined, "data-graph-visual-selection": visualState.selection || undefined, "data-graph-visual-status": visualState.status, "data-graph-visual-surface": visualState.surface, "data-graph-visual-treatment": visualState.statusTreatment, "data-graph-visual-validation": visualState.validation, handles: packageHandles, nodeKind: nodeType, showStateIndicator: false, children: children }), resizeControls ? (_jsx(FactoryGraphNodeResizeControls, { ...resizeControls })) : null, activeHints
                ? workstationZAxisIncompleteHintSlots().map((slot) => (_jsx(ZAxisIncompleteHintOrb, { accessibleLabel: activeHints.accessibleLabel, anchorId: slot.anchorId, title: activeHints.title, top: slot.top }, slot.anchorId)))
                : null] }));
}
function classNames(...values) {
    return values.filter(Boolean).join(" ");
}
export function factoryGraphHandleToneFromId(handleId) {
    if (handleId.includes("resource"))
        return "resource";
    if (handleId.includes("worker-assignment") ||
        handleId.includes("worker-input"))
        return "worker";
    if (handleId.includes("on-continue"))
        return "continue";
    if (handleId.includes("on-failure"))
        return "failure";
    if (handleId.includes("on-rejection"))
        return "rejection";
    if (handleId.includes("approval"))
        return "output";
    if (handleId.includes("output"))
        return "output";
    if (handleId.includes("input"))
        return "input";
    if (handleId.includes("assignment"))
        return "assignment";
    return "default";
}
function workstationZAxisIncompleteHintSlots() {
    const railCount = WORKSTATION_RIGHT_RAIL_ANCHOR_IDS.length;
    return Z_AXIS_INCOMPLETE_HINT_ANCHOR_IDS.map((anchorId) => {
        const index = WORKSTATION_RIGHT_RAIL_ANCHOR_IDS.indexOf(anchorId);
        return {
            anchorId,
            top: `${((index + 1) * 100) / (railCount + 1)}%`,
        };
    });
}
function ZAxisIncompleteHintOrb({ accessibleLabel, anchorId, title, top, }) {
    return (_jsx("span", { "aria-label": accessibleLabel, className: "pointer-events-none absolute top-0 right-0 z-20 flex -translate-y-1/2 translate-x-1 flex-row-reverse", "data-z-axis-incomplete-hint": anchorId, role: "img", style: { top }, title: title, children: _jsx("span", { "aria-hidden": "true", className: "block h-2.5 w-2.5 rounded-full border border-af-danger-border bg-error shadow-[0_0_0_3px_var(--color-error-container)] motion-safe:animate-pulse" }) }));
}
