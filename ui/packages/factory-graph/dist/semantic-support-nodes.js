import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { GraphSemanticIcon } from "./semantic-icon.js";
import { FactoryGraphNodeExpandedContent, FactoryGraphNodeShell, } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, factoryGraphNodeVisualIconClassName, factoryGraphNodeWrappedTextClassName, } from "./semantic-node-style.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
import { factoryGraphUnknownWorkerType, factoryGraphWorkerIconClassName, factoryGraphWorkerIconKind, factoryGraphWorkerProviderKind, factoryGraphWorkerProviderLabel, } from "./worker-icon.js";
/** Original Factory worker node, with host-owned worker selection. */
export function FactoryGraphWorkerNodeView({ data, selected: reactFlowSelected, }) {
    const workerName = resolveWorkerName(data);
    const unknownWorkerType = factoryGraphUnknownWorkerType(data.workerType);
    const providerKind = factoryGraphWorkerProviderKind(data.runnerId);
    const providerLabel = factoryGraphWorkerProviderLabel(providerKind);
    const rawProviderLabel = normalizedRawProviderLabel(data.runnerId);
    const providerAccessibleLabel = providerLabel ?? rawProviderLabel;
    const label = unknownWorkerType
        ? `worker:${workerName} (${unknownWorkerType})`
        : `worker:${workerName}${providerAccessibleLabel ? ` (${providerAccessibleLabel})` : ""}`;
    const workerLabel = semanticLabel("worker", data.locale);
    const workerKindLabel = unknownWorkerType ?? workerLabel;
    const workerIconKind = factoryGraphWorkerIconKind(data.workerType, data.runnerId);
    const selectable = data.onSelectWorker !== undefined;
    const selected = data.selectedWorker || reactFlowSelected;
    const visualState = resolveFactoryGraphVisualState({
        activeFlow: data.activeFlow,
        family: "worker",
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
    });
    const content = (_jsx(FactoryGraphWorkerNodeContent, { data: data, label: label, unknownWorkerType: unknownWorkerType, providerLabel: providerLabel, visualState: visualState, workerIconKind: workerIconKind, workerKindLabel: workerKindLabel, workerLabel: workerLabel, workerName: workerName }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames(factoryGraphNodeSurfaceClassName(unknownWorkerType ? "neutral" : "info"), "justify-center text-left text-on-surface", factoryGraphNodeHoverClassName({
            activeFlow: data.activeFlow,
            muted: data.muted,
            selected,
        })), handles: data.handles, interactionOverlay: data.interactionOverlay, nodeType: "worker", resizeControls: data.resizeControls, visualState: {
            activeFlow: data.activeFlow,
            focused: data.focused,
            muted: data.muted,
            selected,
            validation: data.validationError,
        }, children: selectable ? (_jsx(GraphNodeButton, { "aria-label": selectLabel("worker", unknownWorkerType
                ? `${workerName} (${unknownWorkerType})`
                : providerAccessibleLabel
                    ? `${workerName} (${providerAccessibleLabel})`
                    : workerName, data.locale), "aria-pressed": selected, className: "grid h-full min-h-0 min-w-0 place-content-center gap-0.5 overflow-hidden", "data-selected-worker": selected ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectWorker?.(workerName);
            }, children: content })) : (content) }));
}
function FactoryGraphWorkerNodeContent({ data, label, providerLabel, unknownWorkerType, visualState, workerIconKind, workerKindLabel, workerLabel, workerName, }) {
    return (_jsxs(_Fragment, { children: [_jsxs("span", { "aria-label": label, className: "flex h-full min-h-0 min-w-0 items-center gap-1.5 overflow-hidden", "data-factory-entity-semantic-icon": true, "data-worker-label-zone": true, role: "img", title: label, children: [_jsx("span", { className: "sr-only", children: label }), _jsx(GraphSemanticIcon, { className: classNames("h-3.5 w-3.5 shrink-0", factoryGraphWorkerIconClassName(visualState, unknownWorkerType ? "text-on-surface-variant" : "text-info")), kind: workerIconKind, label: providerLabel ? `${workerLabel} (${providerLabel})` : workerLabel }), _jsxs("span", { className: "grid min-w-0 gap-px overflow-hidden", children: [_jsx("span", { className: factoryGraphNodeWrappedTextClassName(classNames("block overflow-hidden text-[0.62rem] font-bold uppercase leading-none", unknownWorkerType ? "text-on-surface-variant" : "text-info")), "data-worker-kind-label": true, children: workerKindLabel }), _jsx("strong", { className: factoryGraphNodeWrappedTextClassName("block font-mono text-[0.8rem] font-bold leading-tight text-on-surface"), "data-factory-entity-title": true, title: workerName, children: workerName })] })] }), data.expanded === true ? (_jsxs(FactoryGraphNodeExpandedContent, { family: "worker", children: [_jsx("span", { "data-factory-graph-expanded-field": "worker-type", children: data.workerType ?? workerLabel }), data.runnerId ? (_jsx("span", { "data-factory-graph-expanded-field": "runner", children: data.runnerId })) : null] })) : null] }));
}
/** Original Factory work-type node, with host-owned selection and validation. */
export function FactoryGraphWorkTypeNodeView({ data, selected: reactFlowSelected, }) {
    const name = workTypeName(data.place);
    const label = `work-type:${name}`;
    const workTypeLabel = semanticLabel("work-type", data.locale);
    const selectable = data.onSelectWorkType !== undefined;
    const isExpanded = data.expanded === true;
    const selected = (data.selectedWorkType ?? false) || reactFlowSelected;
    const visualState = resolveFactoryGraphVisualState({
        activeFlow: data.activeFlow,
        family: "work-type",
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
    });
    const content = (_jsxs(_Fragment, { children: [_jsxs("span", { "aria-hidden": selectable ? true : undefined, className: "flex min-w-0 items-center gap-1.5 overflow-hidden", "data-work-type-label-zone": true, ...(selectable ? {} : { "aria-label": label, role: "img" }), title: data.validationMessage ?? label, children: [selectable ? null : _jsx("span", { className: "sr-only", children: label }), _jsx(GraphSemanticIcon, { className: classNames("h-3.5 w-3.5 shrink-0", factoryGraphNodeVisualIconClassName(visualState, "text-info")), kind: "work-type", label: workTypeLabel }), _jsxs("span", { className: "grid min-w-0 gap-px overflow-hidden", children: [_jsxs("span", { className: "flex min-w-0 items-start gap-1 overflow-hidden", children: [_jsx("span", { className: factoryGraphNodeWrappedTextClassName("block overflow-hidden text-[0.62rem] font-bold uppercase leading-none text-info"), children: workTypeLabel }), data.isDefaultWorkType ? (_jsx(FactoryGraphNodeBadge, { className: "max-w-full shrink", role: "status", tone: "info", weight: "label", children: defaultWorkTypeLabel(data.locale) })) : null] }), _jsx("strong", { className: factoryGraphNodeWrappedTextClassName("block font-mono text-[0.8rem] font-bold leading-tight text-on-surface"), children: name })] })] }), isExpanded ? (_jsx(FactoryGraphNodeExpandedContent, { family: "work-type", children: _jsx("span", { "data-factory-graph-expanded-field": "place-id", children: data.place.place_id }) })) : null] }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames(factoryGraphNodeSurfaceClassName("info"), "justify-center border-dashed text-left text-on-surface", factoryGraphNodeHoverClassName({
            activeFlow: data.activeFlow,
            muted: data.muted,
            selected,
            validationError: data.validationError,
        })), handles: data.handles, interactionOverlay: data.interactionOverlay, nodeType: "workType", resizeControls: data.resizeControls, visualState: {
            activeFlow: data.activeFlow,
            focused: data.focused,
            muted: data.muted,
            selected,
            validation: data.validationError,
        }, children: selectable ? (_jsx(GraphNodeButton, { "aria-invalid": data.validationError ? true : undefined, "aria-label": selectLabel("work type", name, data.locale), "aria-pressed": selected, className: "grid min-w-0 gap-0.5 overflow-hidden", "data-selected-work-type": selected ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectWorkType?.(name);
            }, children: content })) : (_jsx("div", { className: "grid min-w-0 gap-0.5 overflow-hidden", children: content })) }));
}
/** Original Factory resource node, with host-owned resource selection. */
export function FactoryGraphResourceNodeView({ data, selected: reactFlowSelected, }) {
    const label = resourceName(data.place);
    const resourceLabel = semanticLabel("resource", data.locale);
    const selectable = data.onSelectResource !== undefined;
    const isExpanded = data.expanded === true;
    const selected = data.selectedResource || reactFlowSelected;
    const visualState = resolveFactoryGraphVisualState({
        activeFlow: data.activeFlow,
        family: "resource",
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
    });
    const content = (_jsx(FactoryGraphResourceNodeContent, { label: label, locale: data.locale, place: data.place, resourceLabel: resourceLabel, expanded: isExpanded, tokenCount: data.tokenCount, visualState: visualState }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames(factoryGraphNodeSurfaceClassName("resource"), "justify-center text-left text-on-surface", factoryGraphNodeHoverClassName({
            activeFlow: data.activeFlow,
            muted: data.muted,
            selected,
        })), handles: data.handles, interactionOverlay: data.interactionOverlay, nodeType: "resource", resizeControls: data.resizeControls, visualState: {
            activeFlow: data.activeFlow,
            focused: data.focused,
            muted: data.muted,
            selected,
            validation: data.validationError,
        }, children: selectable ? (_jsx(GraphNodeButton, { "aria-label": selectLabel("resource", label, data.locale), "aria-pressed": selected, className: "flex min-w-0 w-full flex-col overflow-hidden", "data-selected-resource": selected ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectResource?.(label);
            }, children: content })) : (content) }));
}
function FactoryGraphResourceNodeContent({ expanded, label, locale, place, resourceLabel, tokenCount, visualState, }) {
    return (_jsxs("div", { className: "flex min-w-0 w-full flex-col overflow-hidden", children: [_jsxs("span", { "aria-label": label, className: "grid min-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-1.5 overflow-hidden", "data-resource-label-zone": true, role: "img", children: [_jsx("span", { className: "flex min-h-4 shrink-0 items-center", title: resourceLabel, children: _jsx(GraphSemanticIcon, { className: classNames("h-3.5 w-3.5", factoryGraphNodeVisualIconClassName(visualState, "text-success")), kind: "resource", label: resourceLabel }) }), _jsx("span", { className: "flex min-w-0 overflow-hidden", title: label, children: _jsx("span", { className: factoryGraphNodeWrappedTextClassName("block overflow-hidden font-mono text-[0.76rem] font-bold leading-[0.82rem] text-on-surface"), "data-resource-name": true, title: label, children: label }) })] }), _jsx("span", { className: "flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden", "data-resource-token-zone": true, title: label, children: _jsx(FactoryGraphNodeBadge, { "aria-label": tokenCountLabel(place, tokenCount, locale), className: "w-fit", "data-resource-token-count": true, role: "status", children: tokenCount }) }), expanded ? (_jsx(FactoryGraphNodeExpandedContent, { family: "resource", children: _jsx("span", { "data-factory-graph-expanded-field": "place-id", children: place.place_id }) })) : null] }));
}
export function FactoryGraphNodeBadge({ children, className, tone = "neutral", weight = "body", ...rest }) {
    const toneClass = {
        danger: "border-af-danger-border bg-error-container text-on-error-container",
        info: "border-af-info-border bg-info-container text-on-info-container",
        neutral: "border-outline bg-surface-container-low text-on-surface-variant",
        success: "border-af-success-border bg-success-container text-on-success-container",
        warning: "border-af-warning-border bg-warning-container text-on-warning-container",
    }[tone];
    return (_jsx("span", { className: classNames("inline-flex min-h-6 w-fit items-center justify-center gap-1 rounded-full border px-2 py-0.5 font-semibold leading-none", toneClass, weight === "body"
            ? "font-mono text-[0.68rem]"
            : "text-[0.65rem] font-semibold uppercase tracking-[0.08em]", className), ...rest, children: children }));
}
function resolveWorkerName(data) {
    return (data.place.state_value ??
        data.factoryGraphNodeId?.replace(/^worker:/, "") ??
        data.place.place_id.replace(/^place:worker:/, "").replace(/^worker:/, ""));
}
function normalizedRawProviderLabel(providerId) {
    const trimmed = typeof providerId === "string" ? providerId.trim() : "";
    return trimmed.length > 0 ? trimmed : undefined;
}
function workTypeName(place) {
    return typeof place.state_value === "string" &&
        place.state_value.trim().length > 0
        ? place.state_value
        : place.place_id.replace(/^work-type:/, "");
}
function resourceName(place) {
    return typeof place.type_id === "string" && place.type_id.trim().length > 0
        ? place.type_id
        : place.place_id.replace(/:available$/, "");
}
function classNames(...values) {
    return values.filter(Boolean).join(" ");
}
function semanticLabel(kind, locale) {
    const chinese = locale === "zh-CN";
    return kind === "resource"
        ? chinese
            ? "资源"
            : "Resource"
        : kind === "worker"
            ? chinese
                ? "工作者"
                : "Worker"
            : chinese
                ? "工作类型"
                : "Work type";
}
function defaultWorkTypeLabel(locale) {
    return locale === "zh-CN" ? "默认工作类型" : "Default work type";
}
function selectLabel(kind, name, locale) {
    return locale === "zh-CN"
        ? `选择 ${name} ${kind === "work type" ? "工作类型" : kind === "worker" ? "工作者" : "资源"}`
        : `Select ${name} ${kind}`;
}
function tokenCountLabel(_place, count, locale) {
    return locale === "zh-CN"
        ? `${count} 个资源令牌`
        : `${count} resource tokens`;
}
