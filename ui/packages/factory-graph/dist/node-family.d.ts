/**
 * Stable semantic families used by every Factory graph renderer.
 *
 * The family is intentionally separate from a rendered node type. A renderer
 * may use `statePosition` or `workType` as its library-specific type while
 * the Factory graph still has one `work-state` or `work-type` family.
 */
export declare const FACTORY_GRAPH_NODE_FAMILIES: readonly ["constraint", "doc", "resource", "worker", "work-state", "work-type", "workstation"];
export type FactoryGraphNodeFamily = (typeof FACTORY_GRAPH_NODE_FAMILIES)[number];
/** Semantic shape vocabulary; dimensions and CSS are resolved by renderers. */
export type FactoryGraphNodeShape = "constraint" | "document" | "resource" | "worker" | "work-state" | "work-type" | "workstation";
export interface FactoryGraphNodeDimensions {
    readonly height: number;
    readonly width: number;
}
export interface FactoryGraphNodeDimensionBounds {
    readonly maximum: FactoryGraphNodeDimensions;
    readonly minimum: FactoryGraphNodeDimensions;
}
/** The dimensions that a future editor resize affordance may change. */
export interface FactoryGraphNodeResizeAxes {
    readonly height: boolean;
    readonly width: boolean;
}
/** Text surfaces used by the pure fitted-size resolver. */
export type FactoryGraphNodeSizingContent = string | readonly (string | null | undefined)[];
/**
 * The package-owned family contract.
 *
 * `defaultDimensions`, bounds, and axis policy are the stable graph contract.
 * Content fit and authored-size normalization are resolved without changing
 * the family identity or copying this table into a host.
 */
export interface FactoryGraphNodeFamilyRole {
    readonly allowedAxes: FactoryGraphNodeResizeAxes;
    readonly defaultDimensions: FactoryGraphNodeDimensions;
    readonly family: FactoryGraphNodeFamily;
    readonly maximumDimensions: FactoryGraphNodeDimensions;
    readonly minimumDimensions: FactoryGraphNodeDimensions;
    readonly shape: FactoryGraphNodeShape;
}
export type FactoryGraphNodeDimensionSource = "default" | "fitted" | "resolved";
export interface FactoryGraphNodeDimensionResolutionOptions {
    readonly authoredDimensions?: FactoryGraphNodeDimensions | null;
    readonly content?: FactoryGraphNodeSizingContent;
}
/**
 * The complete pure sizing result. Hosts may use the resolved dimensions for
 * projection while retaining authored layout and interaction ownership.
 */
export interface FactoryGraphNodeDimensionResolution {
    readonly allowedAxes: FactoryGraphNodeResizeAxes;
    readonly bounds: FactoryGraphNodeDimensionBounds;
    readonly defaultDimensions: FactoryGraphNodeDimensions;
    readonly fittedDimensions: FactoryGraphNodeDimensions;
    readonly maximumDimensions: FactoryGraphNodeDimensions;
    readonly minimumDimensions: FactoryGraphNodeDimensions;
    readonly resolvedDimensions: FactoryGraphNodeDimensions;
    readonly source: FactoryGraphNodeDimensionSource;
}
/** Read-only family table for package consumers that need the full role. */
export declare const FACTORY_GRAPH_NODE_FAMILY_ROLES: Record<"constraint" | "doc" | "resource" | "worker" | "work-state" | "work-type" | "workstation", FactoryGraphNodeFamilyRole>;
export declare function factoryGraphNodeFamilyRole(family: FactoryGraphNodeFamily): FactoryGraphNodeFamilyRole;
export declare function factoryGraphNodeFamilyDimensions(family: FactoryGraphNodeFamily): FactoryGraphNodeDimensions;
/** Resolve a deterministic, family-bounded size from content or authored data. */
export declare function resolveFactoryGraphNodeDimensions(family: FactoryGraphNodeFamily, request?: FactoryGraphNodeDimensionResolutionOptions | FactoryGraphNodeDimensions | null): FactoryGraphNodeDimensionResolution;
export declare function fitFactoryGraphNodeDimensions(family: FactoryGraphNodeFamily, content?: FactoryGraphNodeSizingContent): FactoryGraphNodeDimensions;
/** Normalize an interactive resize request to the family's safe geometry. */
export declare function resolveFactoryGraphNodeResizeDimensions(family: FactoryGraphNodeFamily, requestedDimensions: FactoryGraphNodeDimensions): FactoryGraphNodeDimensions;
export type FactoryGraphNodeShellType = "constraint" | "doc" | "resource" | "statePosition" | "worker" | "workType" | "workstation";
export declare function factoryGraphNodeFamilyForShellType(nodeType: FactoryGraphNodeShellType): FactoryGraphNodeFamily;
