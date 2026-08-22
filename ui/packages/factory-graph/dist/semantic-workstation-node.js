import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { FactoryGraphNodeShell, } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, factoryGraphNodeWrappedTextClassName, } from "./semantic-node-style.js";
import { FactoryGraphWorkProgressMarker } from "./semantic-work-progress-marker.js";
import { factoryGraphActiveItemsLabel as activeItemsLabel, factoryGraphClassNames as classNames, factoryGraphDurationText as durationText, factoryGraphGraphDuration as graphDuration, factoryGraphSelectWorkstationLabel as selectWorkstationLabel, factoryGraphWorkItemLabel as workItemLabel, factoryGraphWorkItemLabelClassName as workItemLabelClassName, factoryGraphWorkstationPresentation as workstationPresentation, factoryGraphWorkstationTitleClassName as workstationTitleClassName, } from "./semantic-workstation-presentation.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
import { factoryGraphWorkProgressMode } from "./work-progress-presentation.js";
const WORKSTATION_WORK_ITEM_MODE_MAXIMUM = 2;
const WORKSTATION_HEADER_CLASS_NAME = "flex min-w-0 w-full flex-wrap items-start justify-between gap-1 overflow-hidden";
/** Original Factory workstation presentation, with host-owned selection callbacks. */
export function FactoryGraphWorkstationNodeView({ data, selected: reactFlowSelected, }) {
    const presentation = workstationPresentation(data.workstationSemantics, data.locale);
    const title = data.workstation.workstation_name ||
        data.workstation.transition_id ||
        data.workstation.node_id;
    const isExpanded = data.expanded === true;
    const entries = data.executions.flatMap((execution) => (execution.work_items ?? []).map((workItem) => ({ execution, workItem })));
    const selected = data.selectedWorkstation || reactFlowSelected;
    const visualState = resolveFactoryGraphVisualState({
        activeWork: data.active,
        family: "workstation",
        focused: data.focused,
        lifecycle: data.active ? "PROCESSING" : undefined,
        muted: data.muted,
        runtimeStatus: data.runtimeStatus,
        selected,
        validation: data.validationError,
    });
    const className = classNames(factoryGraphNodeSurfaceClassName("workstation"), "min-w-0 w-full justify-start overflow-hidden border-2", factoryGraphNodeHoverClassName({ muted: data.muted, selected }, "primary"), "border-info-border", presentation.borderClassName, data.selectedWorkID !== null &&
        "border-info-border shadow-af-info-selected");
    return (_jsx(FactoryGraphNodeShell, { className: className, handles: data.handles, interactionOverlay: data.interactionOverlay, nodeType: "workstation", resizeControls: data.resizeControls, visualState: {
            activeWork: data.active,
            focused: data.focused,
            lifecycle: data.active ? "PROCESSING" : undefined,
            muted: data.muted,
            runtimeStatus: data.runtimeStatus,
            selected,
            validation: data.validationError,
        }, zAxisIncompleteHints: data.zAxisIncompleteHints, children: data.summaryOnly ? (_jsx(Summary, { data: data, isExpanded: isExpanded, presentation: presentation, title: title, visualState: visualState })) : (_jsx(ActiveContent, { data: data, entries: entries, isExpanded: isExpanded, presentation: presentation, title: title, visualState: visualState })) }));
}
function Summary({ data, isExpanded, presentation, title, visualState, }) {
    const header = (_jsx(Header, { presentation: presentation, showAuxiliaryDetails: isExpanded, title: title }));
    return (_jsxs("div", { className: "grid min-w-0 gap-0.5", "data-workstation-control-role": presentation.controlRole, "data-workstation-density": "compact", "data-workstation-runtime-type": presentation.runtimeType, "data-workstation-scheduling-behavior": presentation.schedulingBehavior, children: [data.onSelectWorkstation ? (_jsx(GraphNodeButton, { "aria-label": selectWorkstationLabel(title, data.locale), "aria-pressed": visualState.selection, className: WORKSTATION_HEADER_CLASS_NAME, "data-selected-workstation": visualState.selection ? "true" : undefined, onClick: (event) => {
                    event.stopPropagation();
                    data.onSelectWorkstation?.(data.workstation.node_id);
                }, title: title, children: header })) : (_jsx("div", { className: WORKSTATION_HEADER_CLASS_NAME, title: title, children: header })), _jsx(FactoryGraphWorkstationGuardedControlCard, { locale: data.locale, presentation: presentation })] }));
}
function ActiveContent({ data, entries, isExpanded, presentation, title, visualState, }) {
    const progressMode = factoryGraphWorkProgressMode(entries.length, WORKSTATION_WORK_ITEM_MODE_MAXIMUM);
    const visible = progressMode === "items" ? entries : [];
    const header = (_jsx(Header, { presentation: presentation, showAuxiliaryDetails: isExpanded, title: title }));
    return (_jsxs("div", { className: "grid h-full min-w-0 grid-rows-[auto_auto_1fr_auto]", "data-active": data.active ? "true" : undefined, "data-selected-work": data.selectedWorkID !== null ? "true" : undefined, "data-selected-workstation": visualState.selection ? "true" : undefined, "data-workstation-control-role": presentation.controlRole, "data-workstation-density": "compact", "data-workstation-runtime-type": presentation.runtimeType, "data-workstation-scheduling-behavior": presentation.schedulingBehavior, children: [data.onSelectWorkstation ? (_jsx(GraphNodeButton, { "aria-label": selectWorkstationLabel(title, data.locale), "aria-pressed": visualState.selection, className: WORKSTATION_HEADER_CLASS_NAME, onClick: (event) => {
                    event.stopPropagation();
                    data.onSelectWorkstation?.(data.workstation.node_id);
                }, title: title, children: header })) : (_jsx("div", { className: WORKSTATION_HEADER_CLASS_NAME, title: title, children: header })), _jsx(FactoryGraphWorkstationGuardedControlCard, { locale: data.locale, presentation: presentation }), _jsx("ul", { className: "mt-1 grid min-h-0 min-w-0 list-none content-start gap-0.5 overflow-hidden p-0", children: visible.map(({ execution, workItem }) => (_jsx(WorkItem, { data: data, execution: execution, workItem: workItem }, `${execution.dispatch_id}:${workItem.work_id}`))) }), _jsx(Overflow, { total: entries.length, visible: visible.length, locale: data.locale })] }));
}
function WorkItem({ data, execution, workItem, }) {
    const selected = data.selectedWorkID === workItem.work_id;
    const label = workItemLabel(workItem);
    const duration = graphDuration(execution.started_at, data.now, data.locale);
    const durationTitle = durationText(execution.started_at, data.now, data.locale);
    const content = (_jsxs(_Fragment, { children: [_jsx("span", { className: classNames(workItemLabelClassName(label), "!block !whitespace-nowrap truncate"), "data-active-work-label": true, children: label }), _jsx("span", { className: "shrink-0 whitespace-nowrap text-right font-mono text-[0.72rem] text-on-surface-subtle", "data-active-work-duration": true, children: duration })] }));
    const className = classNames("grid min-w-0 w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-1 overflow-hidden rounded-lg border border-outline bg-surface px-1.5 py-1 text-[0.74rem]", selected && "border-info-border bg-info-container shadow-af-info-chip");
    return (_jsx("li", { children: data.onSelectWorkID ? (_jsx(GraphNodeButton, { "aria-pressed": selected, className: className, "data-selected": selected ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectWorkID?.(workItem.work_id, {
                    dispatchID: execution.dispatch_id,
                    nodeID: data.workstation.node_id,
                });
            }, title: `${label} - ${durationTitle}`, children: content })) : (_jsx("div", { className: className, "data-selected": selected ? "true" : undefined, title: `${label} - ${durationTitle}`, children: content })) }));
}
function Header({ compact = false, presentation, showAuxiliaryDetails = false, title, }) {
    return (_jsxs(_Fragment, { children: [_jsx("span", { className: compact
                    ? factoryGraphNodeWrappedTextClassName("block font-mono text-[0.74rem] font-bold leading-tight text-on-surface")
                    : workstationTitleClassName(title), "data-factory-entity-title": true, "data-workstation-title": true, title: showAuxiliaryDetails ? undefined : presentation.label, children: title }), showAuxiliaryDetails ? (_jsx("span", { className: factoryGraphNodeWrappedTextClassName("text-[0.62rem] font-semibold leading-tight text-on-surface-subtle"), "data-workstation-runtime-label": true, title: presentation.label, children: presentation.label })) : null, showAuxiliaryDetails && presentation.schedulingLabel ? (_jsx("span", { className: "min-w-0 max-w-full shrink truncate whitespace-nowrap rounded-sm border border-outline-variant bg-surface px-1.5 py-0.5 text-[0.62rem] font-semibold leading-none text-on-surface-subtle", "data-workstation-scheduling-label": true, title: presentation.schedulingLabel, children: presentation.schedulingLabel })) : null] }));
}
export function FactoryGraphWorkstationGuardedControlCard({ locale, presentation, }) {
    const control = presentation.guardedControl;
    if (presentation.controlRole !== "LOOP_BREAKER" || !control)
        return null;
    const label = locale === "zh-CN" ? "断路器" : "Breaker";
    return (_jsx("span", { className: "min-w-0 max-w-full shrink truncate whitespace-nowrap text-[0.62rem] font-semibold leading-none text-on-surface", "data-workstation-control-role": presentation.controlRole, "data-workstation-guard-card": true, "data-workstation-guard-type": control.guardType, title: label, children: label }));
}
function Overflow({ locale, total, visible, }) {
    if (total <= visible)
        return null;
    return (_jsx(FactoryGraphWorkProgressMarker, { ariaLabel: activeItemsLabel(total, locale), className: "mt-1 flex min-h-7 w-full px-3 py-1", count: total, "data-workstation-work-progress": "numeric", kind: "numeric" }));
}
