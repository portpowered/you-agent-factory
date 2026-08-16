import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { FactoryGraphNodeShell, } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, factoryGraphNodeWrappedTextClassName, } from "./semantic-node-style.js";
import { FactoryGraphWorkProgressMarker } from "./semantic-place-nodes.js";
import { factoryGraphActiveItemsLabel as activeItemsLabel, factoryGraphClassNames as classNames, factoryGraphDurationText as durationText, factoryGraphWorkstationControlRoleLabel, factoryGraphWorkstationGuardLimitLabel, factoryGraphWorkstationGuardLimitValue, factoryGraphWorkstationGuardTargetLabel, factoryGraphGraphDuration as graphDuration, factoryGraphSelectWorkstationLabel as selectWorkstationLabel, factoryGraphWorkItemLabel as workItemLabel, factoryGraphWorkItemLabelClassName as workItemLabelClassName, factoryGraphWorkstationPresentation as workstationPresentation, factoryGraphWorkstationTitleClassName as workstationTitleClassName, } from "./semantic-workstation-presentation.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
const VISIBLE_WORK_ITEM_LIMIT = 3;
/** Original Factory workstation presentation, with host-owned selection callbacks. */
export function FactoryGraphWorkstationNodeView({ data, selected: reactFlowSelected, }) {
    const presentation = workstationPresentation(data.workstationSemantics, data.locale);
    const title = data.workstation.workstation_name ||
        data.workstation.transition_id ||
        data.workstation.node_id;
    const entries = data.executions.flatMap((execution) => (execution.work_items ?? []).map((workItem) => ({ execution, workItem })));
    const selected = data.selectedWorkstation || reactFlowSelected;
    const visualState = resolveFactoryGraphVisualState({
        activeFlow: data.activeFlow,
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
    return (_jsx(FactoryGraphNodeShell, { className: className, handles: data.handles, interactionOverlay: data.interactionOverlay, nodeType: "workstation", resizeControls: data.resizeControls
            ? { ...data.resizeControls, isVisible: selected }
            : undefined, visualState: {
            activeFlow: data.activeFlow,
            focused: data.focused,
            lifecycle: data.active ? "PROCESSING" : undefined,
            muted: data.muted,
            runtimeStatus: data.runtimeStatus,
            selected,
            validation: data.validationError,
        }, zAxisIncompleteHints: data.zAxisIncompleteHints, children: data.summaryOnly ? (_jsx(Summary, { data: data, presentation: presentation, title: title, visualState: visualState })) : (_jsx(ActiveContent, { data: data, entries: entries, presentation: presentation, title: title, visualState: visualState })) }));
}
function Summary({ data, presentation, title, visualState, }) {
    return (_jsxs("div", { className: "grid min-w-0 gap-1", "data-workstation-control-role": presentation.controlRole, "data-workstation-runtime-type": presentation.runtimeType, "data-workstation-scheduling-behavior": presentation.schedulingBehavior, children: [_jsx(GraphNodeButton, { "aria-label": data.onSelectWorkstation
                    ? selectWorkstationLabel(title, data.locale)
                    : undefined, "aria-pressed": data.onSelectWorkstation ? visualState.selection : undefined, className: "flex min-w-0 w-full flex-wrap items-start justify-between gap-2 overflow-hidden", "data-selected-workstation": visualState.selection ? "true" : undefined, disabled: data.onSelectWorkstation === undefined, onClick: data.onSelectWorkstation
                    ? (event) => {
                        event.stopPropagation();
                        data.onSelectWorkstation?.(data.workstation.node_id);
                    }
                    : undefined, title: title, children: _jsx(Header, { presentation: presentation, title: title }) }), _jsx(FactoryGraphWorkstationGuardedControlCard, { locale: data.locale, presentation: presentation })] }));
}
function ActiveContent({ data, entries, presentation, title, visualState, }) {
    const visible = entries.slice(0, VISIBLE_WORK_ITEM_LIMIT);
    const header = (_jsx(Header, { presentation: presentation, title: title }));
    return (_jsxs("div", { className: "grid h-full min-w-0 grid-rows-[auto_auto_1fr_auto]", "data-active": data.active ? "true" : undefined, "data-selected-work": data.selectedWorkID !== null ? "true" : undefined, "data-selected-workstation": visualState.selection ? "true" : undefined, "data-workstation-control-role": presentation.controlRole, "data-workstation-runtime-type": presentation.runtimeType, "data-workstation-scheduling-behavior": presentation.schedulingBehavior, children: [data.onSelectWorkstation ? (_jsx(GraphNodeButton, { "aria-label": selectWorkstationLabel(title, data.locale), "aria-pressed": visualState.selection, className: "flex min-w-0 w-full flex-wrap items-start justify-between gap-2 overflow-hidden", onClick: (event) => {
                    event.stopPropagation();
                    data.onSelectWorkstation?.(data.workstation.node_id);
                }, title: title, children: header })) : (_jsx("div", { className: "flex min-w-0 w-full flex-wrap items-start justify-between gap-2 overflow-hidden", title: title, children: header })), _jsx(FactoryGraphWorkstationGuardedControlCard, { locale: data.locale, presentation: presentation }), _jsx("ul", { className: "mt-2 grid min-w-0 list-none content-start gap-1 p-0", children: visible.map(({ execution, workItem }) => (_jsx(WorkItem, { data: data, execution: execution, workItem: workItem }, `${execution.dispatch_id}:${workItem.work_id}`))) }), _jsx(Overflow, { total: entries.length, visible: visible.length, locale: data.locale })] }));
}
function WorkItem({ data, execution, workItem, }) {
    const selected = data.selectedWorkID === workItem.work_id;
    const label = workItemLabel(workItem);
    const duration = graphDuration(execution.started_at, data.now, data.locale);
    const durationTitle = durationText(execution.started_at, data.now, data.locale);
    const content = (_jsxs(_Fragment, { children: [_jsx("span", { className: workItemLabelClassName(label), "data-active-work-label": true, children: label }), _jsx("span", { className: "shrink-0 whitespace-nowrap text-right font-mono text-[0.72rem] text-on-surface-subtle", "data-active-work-duration": true, children: duration })] }));
    const className = classNames("grid min-w-0 w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-1 overflow-hidden rounded-lg border border-outline bg-surface px-1.5 py-1 text-[0.74rem]", selected && "border-info-border bg-info-container shadow-af-info-chip");
    return (_jsx("li", { children: data.onSelectWorkID ? (_jsx(GraphNodeButton, { "aria-pressed": selected, className: className, "data-selected": selected ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectWorkID?.(workItem.work_id, {
                    dispatchID: execution.dispatch_id,
                    nodeID: data.workstation.node_id,
                });
            }, title: `${label} - ${durationTitle}`, children: content })) : (_jsx("div", { className: className, "data-selected": selected ? "true" : undefined, title: `${label} - ${durationTitle}`, children: content })) }));
}
function Header({ compact = false, presentation, title, }) {
    return (_jsxs(_Fragment, { children: [_jsx("span", { className: compact
                    ? factoryGraphNodeWrappedTextClassName("block font-mono text-[0.74rem] font-bold leading-tight text-on-surface")
                    : workstationTitleClassName(title), "data-factory-entity-title": true, "data-workstation-title": true, children: title }), _jsx("span", { className: factoryGraphNodeWrappedTextClassName("text-[0.62rem] font-semibold leading-tight text-on-surface-subtle"), "data-workstation-runtime-label": true, title: presentation.label, children: presentation.label }), presentation.schedulingLabel ? (_jsx("span", { className: factoryGraphNodeWrappedTextClassName("shrink-0 rounded-sm border border-outline-variant bg-surface px-1.5 py-0.5 text-[0.62rem] font-semibold leading-none text-on-surface-subtle"), "data-workstation-scheduling-label": true, title: presentation.schedulingLabel, children: presentation.schedulingLabel })) : null] }));
}
export function FactoryGraphWorkstationGuardedControlCard({ locale, presentation, }) {
    const control = presentation.guardedControl;
    if (presentation.controlRole !== "LOOP_BREAKER" || !control)
        return null;
    const roleLabel = factoryGraphWorkstationControlRoleLabel(presentation.controlRole, locale);
    return (_jsxs("fieldset", { "aria-label": roleLabel, className: "grid min-w-0 gap-1 rounded-md border border-af-warning-border bg-warning-container px-2 py-1.5 text-[0.68rem] text-on-warning-container", "data-workstation-guard-card": true, "data-workstation-guard-type": control.guardType, "data-workstation-control-role": presentation.controlRole, children: [_jsx("span", { className: factoryGraphNodeWrappedTextClassName("font-semibold uppercase tracking-[0.06em]"), "data-workstation-control-role-label": true, children: roleLabel }), _jsxs("dl", { className: "m-0 grid min-w-0 gap-0.5", children: [_jsxs("div", { className: "grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-1", children: [_jsx("dt", { className: "shrink-0", children: factoryGraphWorkstationGuardTargetLabel(locale) }), _jsx("dd", { className: factoryGraphNodeWrappedTextClassName("m-0 font-mono"), "data-workstation-guard-target": true, title: control.targetWorkstation, children: control.targetWorkstation })] }), _jsxs("div", { className: "grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-1", children: [_jsx("dt", { className: "shrink-0", children: factoryGraphWorkstationGuardLimitLabel(locale) }), _jsx("dd", { className: factoryGraphNodeWrappedTextClassName("m-0 font-mono"), "data-workstation-guard-limit": true, title: factoryGraphWorkstationGuardLimitValue(control), children: factoryGraphWorkstationGuardLimitValue(control) })] })] })] }));
}
function Overflow({ locale, total, visible, }) {
    const remaining = Math.max(0, total - visible);
    if (!remaining)
        return null;
    if (remaining > 10)
        return (_jsx(FactoryGraphWorkProgressMarker, { ariaLabel: activeItemsLabel(total, locale), className: "mt-2 flex min-h-7 w-full rounded-lg px-3 py-1 text-[0.9rem]", count: total, "data-workstation-work-progress": "numeric", kind: "numeric" }));
    return (_jsx(FactoryGraphWorkProgressMarker, { ariaLabel: activeItemsLabel(total, locale), className: "mt-2 flex min-h-7 gap-1 rounded-lg px-2", "data-workstation-work-progress": "dots", dotClassName: "h-1.5 w-1.5", dotCount: remaining, dotDataAttribute: "data-workstation-work-progress-dot", kind: "dots", suffix: _jsxs("span", { className: "ml-1 font-mono text-[0.68rem] font-bold text-success", children: ["+", remaining] }) }));
}
