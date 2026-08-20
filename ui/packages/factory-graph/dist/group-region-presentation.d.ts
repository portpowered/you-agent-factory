export declare const FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS: readonly ["neutral", "primary", "info", "success", "warning", "danger", "outline"];
export type FactoryGraphGroupRegionColorToken = (typeof FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS)[number];
export type FactoryGraphGroupRegionResolvedColor = Exclude<FactoryGraphGroupRegionColorToken, "outline"> | `#${string}`;
export interface FactoryGraphGroupRegionBounds {
    height: number;
    width: number;
    x: number;
    y: number;
}
/**
 * Read-only presentation input for a saved layout group.
 *
 * This is intentionally a structural view contract rather than a durable
 * group schema. Hosts can pass the existing Factory layout group directly;
 * persistence remains owned by the host's layout model.
 */
export interface FactoryGraphGroupRegionInput {
    bounds: FactoryGraphGroupRegionBounds;
    color?: string;
    id: string;
    label?: string;
}
export interface FactoryGraphGroupRegionView {
    bounds: FactoryGraphGroupRegionBounds;
    color: FactoryGraphGroupRegionResolvedColor;
    id: string;
    label: string;
}
export interface FactoryGraphGroupRegionColorStyle {
    accent: string;
    fill: string;
}
/** Normalize safe hex colors before using them in inline styles. */
export declare function normalizeFactoryGraphGroupRegionCustomColor(color: string | undefined): `#${string}` | null;
/** Resolve legacy outline and unknown values without interpolating raw CSS. */
export declare function resolveFactoryGraphGroupRegionColor(color: string | undefined): FactoryGraphGroupRegionResolvedColor;
export declare function factoryGraphGroupRegionColorStyle(color: string | undefined): FactoryGraphGroupRegionColorStyle;
export declare function isValidFactoryGraphGroupRegionBounds(bounds: FactoryGraphGroupRegionBounds): boolean;
/** Project saved groups into safe, render-ready read-only view data. */
export declare function projectFactoryGraphGroupRegions(groups: readonly FactoryGraphGroupRegionInput[] | undefined): FactoryGraphGroupRegionView[];
export declare function projectFactoryGraphGroupRegionBounds(bounds: FactoryGraphGroupRegionBounds, transform: readonly [number, number, number]): FactoryGraphGroupRegionBounds;
export interface FactoryGraphGroupRegionLayerProps {
    groupAriaLabel?: (group: FactoryGraphGroupRegionView) => string;
    groups: readonly FactoryGraphGroupRegionInput[] | undefined;
}
/**
 * Render saved groups as decorative graph regions.
 *
 * The layer and every region are pointer-transparent by design. Hosts that
 * need editing compose their own narrowly scoped affordances above this
 * layer; this component never creates edit controls or owns layout state.
 */
export declare function FactoryGraphGroupRegionLayer({ groupAriaLabel, groups, }: FactoryGraphGroupRegionLayerProps): import("react").JSX.Element | null;
