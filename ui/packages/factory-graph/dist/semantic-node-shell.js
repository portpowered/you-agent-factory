import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { GraphNodeShell, } from "@you-agent-factory/components/graphs";
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
export function FactoryGraphNodeShell({ children, className = "", handles, nodeType, zAxisIncompleteHints = null, }) {
    const packageHandles = handles.map((handle) => ({
        ...handle,
        tone: handle.tone ?? factoryGraphHandleToneFromId(handle.id),
    }));
    const activeHints = nodeType === "workstation" ? zAxisIncompleteHints : null;
    return (_jsxs("div", { className: "relative h-full min-w-0 w-full", children: [_jsx(GraphNodeShell, { className: className, "data-current-activity-node-type": nodeType, handles: packageHandles, nodeKind: nodeType, showStateIndicator: false, children: children }), activeHints
                ? workstationZAxisIncompleteHintSlots().map((slot) => (_jsx(ZAxisIncompleteHintOrb, { accessibleLabel: activeHints.accessibleLabel, anchorId: slot.anchorId, title: activeHints.title, top: slot.top }, slot.anchorId)))
                : null] }));
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
