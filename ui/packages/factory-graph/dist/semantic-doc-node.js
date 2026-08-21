import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { GraphSemanticIcon } from "./semantic-icon.js";
import { FactoryGraphNodeShell, } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, factoryGraphNodeVisualIconClassName, factoryGraphNodeWrappedTextClassName, } from "./semantic-node-style.js";
import { resolveFactoryGraphVisualState, } from "./visual-state.js";
/** Original Factory document node, with host-owned selection callback. */
export function FactoryGraphDocNodeView({ data, selected: reactFlowSelected, }) {
    const selectable = data.onSelectDoc !== undefined;
    const docLabel = "Document";
    const selected = data.selectedDoc || reactFlowSelected;
    const visualState = resolveFactoryGraphVisualState({
        activeFlow: data.activeFlow,
        family: "doc",
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
    });
    return (_jsx(FactoryGraphNodeShell, { className: [
            factoryGraphNodeSurfaceClassName("neutral"),
            "justify-center text-left text-on-surface",
            factoryGraphNodeHoverClassName({
                activeFlow: data.activeFlow,
                muted: data.muted,
                selected,
                validationError: data.validationError,
            }),
        ]
            .filter(Boolean)
            .join(" "), handles: data.handles, interactionOverlay: data.interactionOverlay, nodeType: "doc", resizeControls: data.resizeControls, visualState: {
            activeFlow: data.activeFlow,
            focused: data.focused,
            muted: data.muted,
            selected,
            validation: data.validationError,
        }, children: selectable ? (_jsx(GraphNodeButton, { "aria-label": selectDocLabel(data.displayLabel, data.locale), "aria-invalid": data.validationError ? true : undefined, "aria-pressed": selected, className: "grid min-w-0 gap-0.5 overflow-hidden", "data-selected-doc": selected ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectDoc?.(data.targetPath);
            }, children: _jsx(FactoryGraphDocNodeContent, { displayLabel: data.displayLabel, docLabel: docLabel, targetPath: data.targetPath, visualState: visualState }) })) : (_jsx(FactoryGraphDocNodeContent, { displayLabel: data.displayLabel, docLabel: docLabel, targetPath: data.targetPath, visualState: visualState })) }));
}
function FactoryGraphDocNodeContent({ displayLabel, docLabel, targetPath, visualState, }) {
    return (_jsxs("div", { className: "grid min-w-0 gap-1 px-2 py-1", children: [_jsxs("div", { className: "flex min-w-0 items-center gap-1.5", children: [_jsx("span", { "data-factory-entity-semantic-icon": true, children: _jsx(GraphSemanticIcon, { className: factoryGraphNodeVisualIconClassName(visualState, "text-on-surface-variant"), kind: "doc", label: docLabel }) }), _jsx("span", { className: factoryGraphNodeWrappedTextClassName("block text-sm font-medium text-on-surface"), "data-factory-entity-title": true, children: displayLabel })] }), _jsx("span", { className: factoryGraphNodeWrappedTextClassName("block text-xs text-on-surface-variant"), children: targetPath })] }));
}
function selectDocLabel(displayLabel, locale) {
    return locale === "zh-CN"
        ? `选择 ${displayLabel} 文档`
        : `Select ${displayLabel} doc`;
}
