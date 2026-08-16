import { jsx as _jsx } from "react/jsx-runtime";
import { useStore } from "@xyflow/react";
export const FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS = [
    "neutral",
    "primary",
    "info",
    "success",
    "warning",
    "danger",
    "outline",
];
const FACTORY_GRAPH_GROUP_REGION_COLOR_STYLES = {
    danger: {
        accent: "var(--color-error)",
        fill: "var(--color-error-container)",
    },
    info: {
        accent: "var(--color-info)",
        fill: "var(--color-info-container)",
    },
    neutral: {
        accent: "var(--color-outline-variant)",
        fill: "var(--color-surface-container-low)",
    },
    primary: {
        accent: "var(--color-primary)",
        fill: "var(--color-primary-container)",
    },
    success: {
        accent: "var(--color-success)",
        fill: "var(--color-success-container)",
    },
    warning: {
        accent: "var(--color-warning)",
        fill: "var(--color-warning-container)",
    },
};
const SUPPORTED_FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS = new Set([
    "danger",
    "info",
    "neutral",
    "primary",
    "success",
    "warning",
]);
const FACTORY_GRAPH_GROUP_REGION_CUSTOM_COLOR_PATTERN = /^#[0-9a-f]{3}(?:[0-9a-f]{3})?$/i;
/** Normalize safe hex colors before using them in inline styles. */
export function normalizeFactoryGraphGroupRegionCustomColor(color) {
    const normalized = color?.trim().toLowerCase();
    if (normalized === undefined ||
        !FACTORY_GRAPH_GROUP_REGION_CUSTOM_COLOR_PATTERN.test(normalized)) {
        return null;
    }
    if (normalized.length === 4) {
        return `#${normalized
            .slice(1)
            .split("")
            .map((digit) => `${digit}${digit}`)
            .join("")}`;
    }
    return normalized;
}
/** Resolve legacy outline and unknown values without interpolating raw CSS. */
export function resolveFactoryGraphGroupRegionColor(color) {
    if (color === "outline") {
        return "neutral";
    }
    const customColor = normalizeFactoryGraphGroupRegionCustomColor(color);
    if (customColor !== null) {
        return customColor;
    }
    return color !== undefined &&
        SUPPORTED_FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS.has(color)
        ? color
        : "neutral";
}
export function factoryGraphGroupRegionColorStyle(color) {
    const resolvedColor = resolveFactoryGraphGroupRegionColor(color);
    if (resolvedColor.startsWith("#")) {
        return {
            accent: resolvedColor,
            fill: `color-mix(in srgb, ${resolvedColor} 18%, transparent)`,
        };
    }
    return FACTORY_GRAPH_GROUP_REGION_COLOR_STYLES[resolvedColor];
}
export function isValidFactoryGraphGroupRegionBounds(bounds) {
    return (Number.isFinite(bounds.height) &&
        Number.isFinite(bounds.width) &&
        Number.isFinite(bounds.x) &&
        Number.isFinite(bounds.y) &&
        bounds.height > 0 &&
        bounds.width > 0);
}
/** Project saved groups into safe, render-ready read-only view data. */
export function projectFactoryGraphGroupRegions(groups) {
    if (!groups) {
        return [];
    }
    return groups.flatMap((group) => {
        const id = group.id.trim();
        if (!id || !isValidFactoryGraphGroupRegionBounds(group.bounds)) {
            return [];
        }
        return [
            {
                bounds: { ...group.bounds },
                color: resolveFactoryGraphGroupRegionColor(group.color),
                id,
                label: group.label?.trim() || id,
            },
        ];
    });
}
export function projectFactoryGraphGroupRegionBounds(bounds, transform) {
    const [translateX, translateY, zoom] = transform;
    return {
        height: bounds.height * zoom,
        width: bounds.width * zoom,
        x: bounds.x * zoom + translateX,
        y: bounds.y * zoom + translateY,
    };
}
/**
 * Render saved groups as decorative graph regions.
 *
 * The layer and every region are pointer-transparent by design. Hosts that
 * need editing compose their own narrowly scoped affordances above this
 * layer; this component never creates edit controls or owns layout state.
 */
export function FactoryGraphGroupRegionLayer({ groupAriaLabel, groups, }) {
    const transform = useStore((state) => state.transform);
    const regions = projectFactoryGraphGroupRegions(groups);
    if (regions.length === 0) {
        return null;
    }
    return (_jsx("div", { className: "pointer-events-none absolute inset-0 z-0", "data-factory-graph-group-region-layer": "", children: regions.map((region, index) => {
            const projectedBounds = projectFactoryGraphGroupRegionBounds(region.bounds, transform);
            const colorStyle = factoryGraphGroupRegionColorStyle(region.color);
            const accessibleLabel = groupAriaLabel?.(region) ?? (region.label || region.id);
            return (_jsx("section", { "aria-label": accessibleLabel, className: "pointer-events-none absolute overflow-visible rounded-2xl border-2", "data-factory-graph-group-region": region.id, style: {
                    backgroundColor: colorStyle.fill,
                    borderColor: colorStyle.accent,
                    boxShadow: `0 0 0 1px ${colorStyle.fill}, 0 0 1rem ${colorStyle.fill}`,
                    height: projectedBounds.height,
                    left: projectedBounds.x,
                    top: projectedBounds.y,
                    width: projectedBounds.width,
                    zIndex: index,
                }, children: _jsx("span", { className: "pointer-events-none absolute top-0 right-3 left-3 -translate-y-1/2", children: _jsx("span", { "aria-hidden": "true", className: "block max-w-full truncate rounded-lg border px-2 py-1 text-xs font-semibold leading-5 text-on-surface shadow-sm backdrop-blur-sm", "data-factory-graph-group-region-label": "", title: region.label, style: {
                            backgroundColor: colorStyle.fill,
                            borderColor: colorStyle.accent,
                        }, children: region.label }) }) }, region.id));
        }) }));
}
