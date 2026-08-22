import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { factoryGraphNodeWrappedTextClassName } from "./semantic-node-style.js";
export function FactoryGraphNodeInteractionOverlayView({ overlay, }) {
    if (!overlay)
        return null;
    const badges = overlay.badges ?? [];
    if (badges.length === 0 && !overlay.connectionHint)
        return null;
    return (_jsxs("div", { className: "pointer-events-none grid min-w-0 gap-1", "data-graph-interaction-overlay": true, children: [badges.length > 0 ? (_jsx("div", { className: "flex min-w-0 flex-wrap items-center gap-1", children: badges.map((badge) => (_jsx("span", { className: badgeClassName(badge.tone), "data-graph-interaction-badge": true, role: "status", children: badge.label }, `${badge.tone ?? "neutral"}-${String(badge.label)}`))) })) : null, overlay.connectionHint ? (_jsx("p", { className: factoryGraphNodeWrappedTextClassName("m-0 text-[0.65rem] leading-5 text-on-surface-subtle"), "data-graph-interaction-hint": true, children: overlay.connectionHint })) : null] }));
}
function badgeClassName(tone = "neutral") {
    const toneClassName = {
        danger: "border-af-danger-border bg-error-container text-on-error-container",
        info: "border-af-info-border bg-info-container text-on-info-container",
        neutral: "border-outline bg-surface-container-low text-on-surface-variant",
        success: "border-af-success-border bg-success-container text-on-success-container",
        warning: "border-af-warning-border bg-warning-container text-on-warning-container",
    }[tone];
    return [
        "inline-flex min-h-6 w-fit items-center justify-center rounded-full border px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
        toneClassName,
    ].join(" ");
}
