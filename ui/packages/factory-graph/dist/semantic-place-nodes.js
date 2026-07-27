import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { GraphSemanticIcon } from "./semantic-icon.js";
import { FactoryGraphNodeShell, } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, } from "./semantic-node-style.js";
import { FactoryGraphNodeBadge, } from "./semantic-support-nodes.js";
import { workStatePhaseSemanticIconClassName, workStatePhaseSemanticIconKind, workStatePhaseSurfaceClassName, } from "./work-state-presentation.js";
const DOT_LIMIT = 10;
const CONTENT_CLASS = "flex min-w-0 w-full flex-col gap-0.5 overflow-hidden";
export function FactoryGraphStatePositionNodeView(props) {
    return _jsx(FactoryGraphPlaceNodeView, { ...props });
}
export function FactoryGraphConstraintNodeView(props) {
    return _jsx(FactoryGraphPlaceNodeView, { ...props });
}
function FactoryGraphPlaceNodeView({ data }) {
    const placeLabel = formatPlaceLabel(data.place);
    const selectable = data.place.kind === "work_state" && data.onSelectStateNode !== undefined;
    const stateNode = data.place.kind === "work_state";
    const nodeType = stateNode
        ? "statePosition"
        : data.place.kind === "resource"
            ? "resource"
            : "constraint";
    const className = classNames(placeNodeClassName(data.place), factoryGraphNodeHoverClassName({
        activeFlow: data.activeFlow,
        muted: data.muted,
        selected: data.selectedStateNode,
        validationError: data.validationError,
    }), data.activeFlow &&
        !data.selectedStateNode &&
        !data.validationError &&
        "border-af-success-border shadow-af-success-chip", data.selectedStateNode &&
        !data.validationError &&
        "border-primary shadow-af-accent-selected", data.validationError &&
        "ring-2 ring-af-danger-border motion-safe:animate-pulse", data.muted && "opacity-[0.45]");
    const content = stateNode ? (_jsx(FactoryGraphStatePositionContent, { locale: data.locale, place: data.place, tokenCount: data.tokenCount })) : (_jsx(FactoryGraphStaticPlaceContent, { locale: data.locale, place: data.place, tokenCount: data.tokenCount }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames("justify-center text-left", className), handles: data.handles, nodeType: nodeType, children: selectable ? (_jsx(GraphNodeButton, { "aria-invalid": data.validationError ? true : undefined, "aria-label": data.validationMessage ?? selectStateLabel(placeLabel, data.locale), "aria-pressed": data.selectedStateNode, className: CONTENT_CLASS, "data-selected-state": data.selectedStateNode ? "true" : undefined, title: data.validationMessage, onClick: (event) => {
                event.stopPropagation();
                data.onSelectStateNode?.(data.place.place_id);
            }, children: content })) : (content) }));
}
function FactoryGraphStatePositionContent({ locale, place, tokenCount, }) {
    const label = formatPlaceLabel(place);
    return (_jsxs(_Fragment, { children: [_jsxs("span", { className: "grid h-6 max-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden", "data-state-label-zone": true, children: [_jsx(FactoryGraphPlaceSemanticIcon, { locale: locale, place: place }), _jsx(FactoryGraphPlaceLabelText, { dataPrefix: "state", place: place })] }), _jsx("span", { className: "flex min-h-5 w-full shrink-0 items-center justify-center overflow-hidden", "data-state-marker-zone": true, title: label, children: stateMarkers(tokenCount, locale) ?? (_jsx("span", { className: "sr-only", children: activeItemCountLabel(tokenCount, locale) })) })] }));
}
function FactoryGraphStaticPlaceContent({ locale, place, tokenCount, }) {
    const label = formatPlaceLabel(place);
    if (place.kind !== "resource")
        return (_jsxs("div", { className: "grid min-w-0 gap-0.5 overflow-hidden", "data-place-label-container": true, children: [_jsxs("span", { className: "flex min-w-0 items-center gap-1.5 overflow-hidden", "data-place-label-zone": true, title: label, children: [_jsx(FactoryGraphPlaceSemanticIcon, { locale: locale, place: place }), _jsx("strong", { className: "block min-w-0 truncate whitespace-nowrap font-mono text-[0.86rem] font-bold leading-tight", children: label })] }), _jsx("span", { className: "flex min-h-4 w-full shrink-0 items-center justify-start overflow-hidden", "data-place-marker-zone": true, title: label, children: tokenCountDisplay(place, tokenCount, locale) })] }));
    return (_jsxs("div", { className: "flex min-w-0 w-full flex-col overflow-hidden", "data-place-label-container": true, children: [_jsxs("span", { "aria-label": label, className: "grid h-6 max-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden", "data-place-label-zone": true, role: "img", children: [_jsx(FactoryGraphPlaceSemanticIcon, { locale: locale, place: place }), _jsx(FactoryGraphPlaceLabelText, { dataPrefix: "place", place: place })] }), _jsx("span", { className: "flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden", "data-place-marker-zone": true, title: label, children: tokenCountDisplay(place, tokenCount, locale) })] }));
}
function FactoryGraphPlaceSemanticIcon({ locale, place, }) {
    const kind = place.kind === "work_state"
        ? workStatePhaseSemanticIconKind(place.state_category)
        : place.kind === "resource"
            ? "resource"
            : place.kind === "limit"
                ? "limit"
                : "constraint";
    const className = place.kind === "work_state"
        ? workStatePhaseSemanticIconClassName(place.state_category)
        : place.kind === "resource"
            ? "text-success"
            : place.kind === "limit"
                ? "text-error"
                : "text-info";
    const label = placeSemanticLabel(place, locale);
    return (_jsx("span", { className: "flex min-h-4 shrink-0 items-center", "data-place-semantic-icon": true, title: placeKindLabel(place, locale), children: _jsx(GraphSemanticIcon, { className: classNames("h-3.5 w-3.5", className), kind: kind, label: label }) }));
}
function FactoryGraphPlaceLabelText({ dataPrefix, place, }) {
    const label = formatPlaceLabel(place);
    const parts = placeLabelParts(place);
    return (_jsxs("span", { className: "grid min-w-0 gap-px overflow-hidden", title: label, children: [_jsx("span", { className: "block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-on-surface-subtle", "data-place-work-type": dataPrefix === "place" ? true : undefined, "data-state-work-type": dataPrefix === "state" ? true : undefined, title: parts.workType, children: parts.workType }), _jsx("span", { className: "block min-w-0 overflow-hidden truncate whitespace-nowrap font-mono text-[0.76rem] font-bold leading-[0.82rem] text-on-surface", "data-place-state-value": dataPrefix === "place" ? true : undefined, "data-state-value": dataPrefix === "state" ? true : undefined, title: parts.stateValue, children: parts.stateValue })] }));
}
function stateMarkers(count, locale) {
    if (count === 0)
        return null;
    return count > DOT_LIMIT ? (_jsx(FactoryGraphWorkProgressMarker, { ariaLabel: activeItemCountLabel(count, locale), className: "inline-flex min-h-5 min-w-7 rounded-full px-2 text-[0.76rem]", count: count, "data-state-work-progress": "numeric", kind: "numeric" })) : (_jsx(FactoryGraphWorkProgressMarker, { ariaLabel: activeItemCountLabel(count, locale), className: "inline-grid grid-cols-[repeat(5,0.5rem)] justify-center gap-1", "data-state-work-progress": "dots", dotCount: count, dotDataAttribute: "data-state-work-progress-dot", kind: "dots" }));
}
export function FactoryGraphWorkProgressMarker(props) {
    if (props.kind === "numeric") {
        const { ariaLabel, className, count, kind: _kind, ...rest } = props;
        return (_jsx("span", { "aria-label": ariaLabel, className: classNames("items-center justify-center border border-af-success-border bg-success-container font-mono font-bold leading-none text-success", className), role: "status", ...rest, children: count }));
    }
    const { ariaLabel, className, dotClassName = "h-2 w-2", dotCount, dotDataAttribute, kind: _kind, suffix, ...rest } = props;
    return (_jsxs("span", { "aria-label": ariaLabel, className: classNames("items-center justify-center border border-af-success-border bg-success-container", className), role: "status", ...rest, children: [Array.from({ length: dotCount }, (_, index) => `dot-${index}`).map((key, index) => (_jsx("span", { "aria-hidden": "true", className: classNames("rounded-full bg-success", dotClassName), "data-current-activity-work-progress-dot": String(index), [dotDataAttribute]: String(index) }, key))), suffix] }));
}
function tokenCountDisplay(place, count, locale) {
    return (_jsx(FactoryGraphNodeBadge, { "aria-label": tokenCountLabel(place, count, locale), className: "w-fit", "data-place-token-count": true, role: "status", children: count }));
}
function placeNodeClassName(place) {
    return place.kind === "work_state"
        ? workStatePhaseSurfaceClassName(place.state_category)
        : place.kind === "resource"
            ? classNames(factoryGraphNodeSurfaceClassName("resource"), "text-on-surface")
            : classNames(factoryGraphNodeSurfaceClassName("info"), "border-dashed text-on-surface");
}
function formatPlaceLabel(place) {
    return place.type_id && place.state_value
        ? `${place.type_id}:${place.state_value}`
        : place.place_id;
}
function placeLabelParts(place) {
    return {
        stateValue: place.state_value ?? place.place_id,
        workType: place.type_id ?? "work",
    };
}
function classNames(...values) {
    return values.filter(Boolean).join(" ");
}
function activeItemCountLabel(count, locale) {
    return locale === "zh-CN"
        ? `${count} 个活动项`
        : `${count} active ${count === 1 ? "item" : "items"}`;
}
function selectStateLabel(label, locale) {
    return locale === "zh-CN" ? `选择 ${label} 状态` : `Select ${label} state`;
}
function placeKindLabel(place, locale) {
    const chinese = locale === "zh-CN";
    if (place.kind === "work_state")
        return place.state_category === "TERMINAL"
            ? chinese
                ? "终止状态"
                : "Terminal"
            : place.state_category === "FAILED"
                ? chinese
                    ? "失败状态"
                    : "Failed"
                : chinese
                    ? "队列"
                    : "Queue";
    if (place.kind === "resource")
        return chinese ? "资源" : "Resource";
    return place.kind === "limit"
        ? chinese
            ? "限制"
            : "Limit"
        : chinese
            ? "约束"
            : "Constraint";
}
function placeSemanticLabel(place, locale) {
    return place.kind === "work_state" && place.state_category === "PROCESSING"
        ? locale === "zh-CN"
            ? "处理中状态"
            : "Processing state"
        : placeKindLabel(place, locale);
}
function tokenCountLabel(place, count, locale) {
    if (locale === "zh-CN")
        return `${count} 个${placeKindLabel(place, locale)}令牌`;
    const token = count === 1 ? "token" : "tokens";
    return `${count} ${placeKindLabel(place, locale).toLowerCase()} ${token}`;
}
