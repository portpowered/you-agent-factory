import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { FactoryGraphNodeExpandedContent, FactoryGraphNodeShell, } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, factoryGraphNodeWrappedTextClassName, } from "./semantic-node-style.js";
import { FactoryGraphPlaceLabelText, FactoryGraphPlaceSemanticIcon, factoryGraphPlaceKindLabel, factoryGraphPlaceLabel, } from "./semantic-place-content.js";
import { FactoryGraphPlaceTokenCount } from "./semantic-place-token-count.js";
import { FactoryGraphWorkProgressMarker } from "./semantic-work-progress-marker.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
import { factoryGraphWorkProgressMode } from "./work-progress-presentation.js";
import { factoryGraphUnknownWorkStateType, isFactoryGraphKnownWorkStateType, workStatePhaseSurfaceClassName, } from "./work-state-presentation.js";
const CONTENT_CLASS = "flex min-w-0 w-full flex-col gap-0.5 overflow-hidden";
export function FactoryGraphStatePositionNodeView(props) {
    return _jsx(FactoryGraphPlaceNodeView, { ...props });
}
export function FactoryGraphConstraintNodeView(props) {
    return _jsx(FactoryGraphPlaceNodeView, { ...props });
}
function FactoryGraphPlaceNodeView({ data, selected: reactFlowSelected, }) {
    const placeLabel = factoryGraphPlaceLabel(data.place);
    const selected = data.selectedStateNode || reactFlowSelected;
    const selectable = data.place.kind === "work_state" && data.onSelectStateNode !== undefined;
    const stateNode = data.place.kind === "work_state";
    const isExpanded = data.expanded === true;
    const nodeType = stateNode
        ? "statePosition"
        : data.place.kind === "resource"
            ? "resource"
            : "constraint";
    const canonicalWorkStateType = stateNode && isFactoryGraphKnownWorkStateType(data.place.state_category)
        ? data.place.state_category
        : undefined;
    const className = classNames(placeNodeClassName(data.place), factoryGraphNodeHoverClassName({
        activeFlow: data.activeFlow,
        muted: data.muted,
        selected,
        validationError: data.validationError,
    }));
    const holdsWork = data.tokenCount > 0;
    const visualState = resolveFactoryGraphVisualState({
        activeFlow: data.activeFlow,
        activeWork: holdsWork,
        family: stateNode
            ? "work-state"
            : data.place.kind === "resource"
                ? "resource"
                : "constraint",
        focused: data.focused,
        lifecycle: canonicalWorkStateType,
        muted: data.muted,
        selected,
        validation: data.validationError,
    });
    const content = stateNode ? (_jsx(FactoryGraphStatePositionContent, { activeItemLabels: data.activeItemLabels, expanded: isExpanded, locale: data.locale, place: data.place, tokenCount: data.tokenCount, visualState: visualState })) : (_jsx(FactoryGraphStaticPlaceContent, { expanded: isExpanded, locale: data.locale, place: data.place, tokenCount: data.tokenCount, visualState: visualState }));
    return (_jsx(FactoryGraphNodeShell, { className: classNames("justify-center text-left", className), handles: data.handles, interactionOverlay: data.interactionOverlay, nodeType: nodeType, resizeControls: data.resizeControls, visualState: {
            activeFlow: data.activeFlow,
            activeWork: holdsWork,
            focused: data.focused,
            lifecycle: canonicalWorkStateType,
            muted: data.muted,
            selected,
            validation: data.validationError,
        }, children: selectable ? (_jsx(GraphNodeButton, { "aria-invalid": data.validationError ? true : undefined, "aria-label": selectStateLabelWithValidation(placeLabel, data.locale, data.place.state_category, data.validationMessage), "aria-pressed": selected, className: CONTENT_CLASS, "data-selected-state": selected ? "true" : undefined, title: data.validationMessage, onClick: (event) => {
                event.stopPropagation();
                data.onSelectStateNode?.(data.place.place_id);
            }, children: content })) : (content) }));
}
function FactoryGraphStatePositionContent({ activeItemLabels, expanded, locale, place, tokenCount, visualState, }) {
    const label = factoryGraphPlaceLabel(place);
    const unknownWorkStateType = factoryGraphUnknownWorkStateType(place.state_category);
    return (_jsxs(_Fragment, { children: [_jsxs("span", { className: "grid min-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-1.5 overflow-hidden", "data-state-label-zone": true, children: [_jsx(FactoryGraphPlaceSemanticIcon, { locale: locale, place: place, visualState: visualState }), _jsxs("span", { className: "grid min-w-0 gap-px overflow-hidden", children: [unknownWorkStateType ? (_jsx("span", { className: "block overflow-hidden text-[0.62rem] font-bold uppercase leading-none text-on-surface-variant", "data-state-category-label": true, title: unknownWorkStateType, children: unknownWorkStateType })) : null, _jsx(FactoryGraphPlaceLabelText, { dataPrefix: "state", place: place })] })] }), _jsx("span", { className: "flex min-h-5 w-full shrink-0 items-center justify-center overflow-hidden", "data-state-marker-zone": true, title: label, children: stateMarkers(tokenCount, locale, visualState) ?? (_jsx("span", { className: "sr-only", children: activeItemCountLabel(tokenCount, locale) })) }), expanded ? (_jsx(FactoryGraphNodeExpandedContent, { family: "work-state", children: _jsx("span", { "data-factory-graph-expanded-field": "active-items", children: activeItemLabels.length > 0
                        ? activeItemLabels.join(", ")
                        : activeItemCountLabel(tokenCount, locale) }) })) : null] }));
}
function FactoryGraphStaticPlaceContent({ expanded, locale, place, tokenCount, visualState, }) {
    const label = factoryGraphPlaceLabel(place);
    if (place.kind !== "resource")
        return (_jsxs("div", { className: "grid min-w-0 gap-0.5 overflow-hidden", "data-place-label-container": true, children: [_jsxs("span", { className: "flex min-w-0 items-center gap-1.5 overflow-hidden", "data-place-label-zone": true, title: label, children: [_jsx(FactoryGraphPlaceSemanticIcon, { locale: locale, place: place, visualState: visualState }), _jsx("strong", { className: factoryGraphNodeWrappedTextClassName("block font-mono text-[0.86rem] font-bold leading-tight"), children: label })] }), _jsx("span", { className: "flex min-h-4 w-full shrink-0 items-center justify-start overflow-hidden", "data-place-marker-zone": true, title: label, children: _jsx(FactoryGraphPlaceTokenCount, { ariaLabel: tokenCountLabel(place, tokenCount, locale), count: tokenCount }) }), expanded ? (_jsx(FactoryGraphNodeExpandedContent, { family: "constraint", children: _jsx("span", { "data-factory-graph-expanded-field": "place-id", children: place.place_id }) })) : null] }));
    return (_jsxs("div", { className: "flex min-w-0 w-full flex-col overflow-hidden", "data-place-label-container": true, children: [_jsxs("span", { "aria-label": label, className: "grid min-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-1.5 overflow-hidden", "data-place-label-zone": true, role: "img", children: [_jsx(FactoryGraphPlaceSemanticIcon, { locale: locale, place: place, visualState: visualState }), _jsx(FactoryGraphPlaceLabelText, { dataPrefix: "place", place: place })] }), _jsx("span", { className: "flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden", "data-place-marker-zone": true, title: label, children: _jsx(FactoryGraphPlaceTokenCount, { ariaLabel: tokenCountLabel(place, tokenCount, locale), count: tokenCount }) }), expanded ? (_jsx(FactoryGraphNodeExpandedContent, { family: "resource", children: _jsx("span", { "data-factory-graph-expanded-field": "place-id", children: place.place_id }) })) : null] }));
}
function stateMarkers(count, locale, visualState) {
    const mode = factoryGraphWorkProgressMode(count);
    if (mode === "empty")
        return null;
    return mode === "total" ? (_jsx(FactoryGraphWorkProgressMarker, { ariaLabel: activeItemCountLabel(count, locale), className: "min-w-8 px-2", count: count, "data-state-work-progress": "numeric", kind: "numeric" })) : (_jsx(FactoryGraphWorkProgressMarker, { ariaLabel: activeItemCountLabel(count, locale), className: "inline-grid grid-cols-[repeat(5,0.5rem)] justify-center gap-1", "data-state-work-progress": "dots", dotCount: count, dotDataAttribute: "data-state-work-progress-dot", active: visualState.surface === "active", kind: "dots" }));
}
function placeNodeClassName(place) {
    return place.kind === "work_state"
        ? workStatePhaseSurfaceClassName(place.state_category)
        : place.kind === "resource"
            ? classNames(factoryGraphNodeSurfaceClassName("resource"), "text-on-surface")
            : classNames(factoryGraphNodeSurfaceClassName("info"), "border-dashed text-on-surface");
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
function selectStateLabelWithValidation(label, locale, stateCategory, validationMessage) {
    const unknownWorkStateType = factoryGraphUnknownWorkStateType(stateCategory);
    const selectionLabel = selectStateLabel(label, locale);
    const compatibilityLabel = unknownWorkStateType
        ? `${selectionLabel} (${unknownWorkStateType})`
        : selectionLabel;
    return validationMessage
        ? `${validationMessage} · ${compatibilityLabel}`
        : compatibilityLabel;
}
function tokenCountLabel(place, count, locale) {
    if (locale === "zh-CN")
        return `${count} 个${factoryGraphPlaceKindLabel(place, locale)}令牌`;
    const token = count === 1 ? "token" : "tokens";
    return `${count} ${factoryGraphPlaceKindLabel(place, locale).toLowerCase()} ${token}`;
}
