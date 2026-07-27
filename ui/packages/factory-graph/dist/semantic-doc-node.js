import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { GraphSemanticIcon } from "./semantic-icon.js";
import { FactoryGraphNodeShell } from "./semantic-node-shell.js";
import { factoryGraphNodeHoverClassName, factoryGraphNodeSurfaceClassName, } from "./semantic-node-style.js";
/** Original Factory document node, with host-owned selection callback. */
export function FactoryGraphDocNodeView({ data, }) {
    const selectable = data.onSelectDoc !== undefined;
    const docLabel = "Document";
    return (_jsx(FactoryGraphNodeShell, { className: [
            factoryGraphNodeSurfaceClassName("neutral"),
            "justify-center text-left text-on-surface",
            factoryGraphNodeHoverClassName({ selected: data.selectedDoc }),
            data.selectedDoc && "border-primary shadow-af-accent-selected",
        ]
            .filter(Boolean)
            .join(" "), handles: data.handles, nodeType: "doc", children: selectable ? (_jsx(GraphNodeButton, { "aria-label": selectDocLabel(data.displayLabel, data.locale), "aria-pressed": data.selectedDoc, className: "grid min-w-0 gap-0.5 overflow-hidden", "data-selected-doc": data.selectedDoc ? "true" : undefined, onClick: (event) => {
                event.stopPropagation();
                data.onSelectDoc?.(data.targetPath);
            }, children: _jsx(FactoryGraphDocNodeContent, { displayLabel: data.displayLabel, docLabel: docLabel, targetPath: data.targetPath }) })) : (_jsx(FactoryGraphDocNodeContent, { displayLabel: data.displayLabel, docLabel: docLabel, targetPath: data.targetPath })) }));
}
function FactoryGraphDocNodeContent({ displayLabel, docLabel, targetPath, }) {
    return (_jsxs("div", { className: "grid min-w-0 gap-1 px-2 py-1", children: [_jsxs("div", { className: "flex min-w-0 items-center gap-1.5", children: [_jsx(GraphSemanticIcon, { className: "text-on-surface-variant", kind: "doc", label: docLabel }), _jsx("span", { className: "truncate text-sm font-medium text-on-surface", children: displayLabel })] }), _jsx("span", { className: "truncate text-xs text-on-surface-variant", children: targetPath })] }));
}
function selectDocLabel(displayLabel, locale) {
    return locale === "zh-CN"
        ? `选择 ${displayLabel} 文档`
        : `Select ${displayLabel} doc`;
}
