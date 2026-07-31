import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { GraphSemanticIcon } from "./semantic-icon.js";
import { FactoryGraphNodeShell, } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, } from "./semantic-node-style.js";
/** Original Factory worker node, with host-owned worker selection. */
export function FactoryGraphWorkerNodeView({ data, }) {
    const workerName = resolveWorkerName(data);
    const label = `worker:${workerName}`;
    const workerLabel = semanticLabel("worker", data.locale);
    const selectable = data.onSelectWorker !== undefined;
    const content = (_jsxs("span", { "aria-label": label, className: "flex min-w-0 items-center gap-1.5 overflow-hidden", "data-worker-label-zone": true, role: "img", title: label, children: [_jsx("span", { className: "sr-only", children: label }), _jsx(GraphSemanticIcon, { className: "h-3.5 w-3.5 shrink-0 text-info", kind: "worker", label: workerLabel }), _jsxs("span", { className: "grid min-w-0 gap-px overflow-hidden", children: [_jsx("span", { className: "block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-info", children: workerLabel }), _jsx("strong", { className: "block min-w-0 truncate whitespace-nowrap font-mono text-[0.8rem] font-bold leading-tight text-on-surface", children: workerName })] })] }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames(factoryGraphNodeSurfaceClassName("info"), "justify-center text-left text-on-surface", factoryGraphNodeHoverClassName({
            activeFlow: data.activeFlow,
            muted: data.muted,
            selected: data.selectedWorker,
        }), data.activeFlow &&
            !data.selectedWorker &&
            "border-af-success-border shadow-af-success-chip", data.selectedWorker && "border-primary shadow-af-accent-selected", data.muted && "opacity-[0.45]"), handles: data.handles, nodeType: "worker", children: selectable ? (_jsx(GraphNodeButton, { "aria-label": selectLabel("worker", workerName, data.locale), "aria-pressed": data.selectedWorker, className: "grid min-w-0 gap-0.5 overflow-hidden", "data-selected-worker": data.selectedWorker ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectWorker?.(workerName);
            }, children: content })) : (content) }));
}
/** Original Factory work-type node, with host-owned selection and validation. */
export function FactoryGraphWorkTypeNodeView({ data, }) {
    const name = workTypeName(data.place);
    const label = `work-type:${name}`;
    const workTypeLabel = semanticLabel("work-type", data.locale);
    const selectable = data.onSelectWorkType !== undefined;
    const content = (_jsxs("span", { "aria-hidden": selectable ? true : undefined, className: "flex min-w-0 items-center gap-1.5 overflow-hidden", "data-work-type-label-zone": true, ...(selectable ? {} : { "aria-label": label, role: "img" }), title: data.validationMessage ?? label, children: [selectable ? null : _jsx("span", { className: "sr-only", children: label }), _jsx(GraphSemanticIcon, { className: "h-3.5 w-3.5 shrink-0 text-info", kind: "work-type", label: workTypeLabel }), _jsxs("span", { className: "grid min-w-0 gap-px overflow-hidden", children: [_jsxs("span", { className: "flex min-w-0 items-center gap-1 overflow-hidden", children: [_jsx("span", { className: "block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-info", children: workTypeLabel }), data.isDefaultWorkType ? (_jsx(FactoryGraphNodeBadge, { className: "max-w-full shrink", role: "status", tone: "info", weight: "label", children: defaultWorkTypeLabel(data.locale) })) : null] }), _jsx("strong", { className: "block min-w-0 truncate whitespace-nowrap font-mono text-[0.8rem] font-bold leading-tight text-on-surface", children: name })] })] }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames(factoryGraphNodeSurfaceClassName("info"), "justify-center border-dashed text-left text-on-surface", factoryGraphNodeHoverClassName({
            activeFlow: data.activeFlow,
            muted: data.muted,
            selected: data.selectedWorkType,
            validationError: data.validationError,
        }), data.activeFlow && "border-info shadow-af-info-chip", data.muted && "opacity-[0.45]", data.validationError &&
            "ring-2 ring-af-danger-border motion-safe:animate-pulse", data.selectedWorkType &&
            !data.validationError &&
            "border-primary shadow-af-accent-selected"), handles: data.handles, nodeType: "workType", children: selectable ? (_jsx(GraphNodeButton, { "aria-invalid": data.validationError ? true : undefined, "aria-label": selectLabel("work type", name, data.locale), "aria-pressed": data.selectedWorkType, className: "grid min-w-0 gap-0.5 overflow-hidden", "data-selected-work-type": data.selectedWorkType ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectWorkType?.(name);
            }, children: content })) : (_jsx("div", { className: "grid min-w-0 gap-0.5 overflow-hidden", children: content })) }));
}
/** Original Factory resource node, with host-owned resource selection. */
export function FactoryGraphResourceNodeView({ data, }) {
    const label = resourceName(data.place);
    const resourceLabel = semanticLabel("resource", data.locale);
    const selectable = data.onSelectResource !== undefined;
    const content = (_jsx(FactoryGraphResourceNodeContent, { label: label, locale: data.locale, place: data.place, resourceLabel: resourceLabel, tokenCount: data.tokenCount }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames(factoryGraphNodeSurfaceClassName("resource"), "justify-center text-left text-on-surface", factoryGraphNodeHoverClassName({
            activeFlow: data.activeFlow,
            muted: data.muted,
            selected: data.selectedResource,
        }), data.activeFlow &&
            !data.selectedResource &&
            "border-af-success-border shadow-af-success-chip", data.selectedResource && "border-primary shadow-af-accent-selected", data.muted && "opacity-[0.45]"), handles: data.handles, nodeType: "resource", children: selectable ? (_jsx(GraphNodeButton, { "aria-label": selectLabel("resource", label, data.locale), "aria-pressed": data.selectedResource, className: "flex min-w-0 w-full flex-col overflow-hidden", "data-selected-resource": data.selectedResource ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectResource?.(label);
            }, children: content })) : (content) }));
}
function FactoryGraphResourceNodeContent({ label, locale, place, resourceLabel, tokenCount, }) {
    return (_jsxs("div", { className: "flex min-w-0 w-full flex-col overflow-hidden", children: [_jsxs("span", { "aria-label": label, className: "grid h-6 max-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden", "data-resource-label-zone": true, role: "img", children: [_jsx("span", { className: "flex min-h-4 shrink-0 items-center", title: resourceLabel, children: _jsx(GraphSemanticIcon, { className: "h-3.5 w-3.5 text-success", kind: "resource", label: resourceLabel }) }), _jsx("span", { className: "flex min-w-0 overflow-hidden", title: label, children: _jsx("span", { className: "block min-w-0 overflow-hidden truncate whitespace-nowrap font-mono text-[0.76rem] font-bold leading-[0.82rem] text-on-surface", "data-resource-name": true, title: label, children: label }) })] }), _jsx("span", { className: "flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden", "data-resource-token-zone": true, title: label, children: _jsx(FactoryGraphNodeBadge, { "aria-label": tokenCountLabel(place, tokenCount, locale), className: "w-fit", "data-resource-token-count": true, role: "status", children: tokenCount }) })] }));
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
