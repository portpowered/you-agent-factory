export declare const GRAPH_SEMANTIC_ICON_KINDS: readonly ["queue", "processing", "terminal", "failed", "resource", "worker", "work-type", "constraint", "doc", "limit", "workstation", "repeater", "cron", "poller", "exhaustion", "active-work"];
export type GraphSemanticIconKind = (typeof GRAPH_SEMANTIC_ICON_KINDS)[number];
export interface GraphSemanticIconProps {
    className?: string;
    kind: GraphSemanticIconKind | (string & {});
    label?: string;
    locale?: string;
}
export declare function graphSemanticIconLabel(kind: GraphSemanticIconProps["kind"]): string;
export declare function GraphSemanticIcon({ className, kind, label, }: GraphSemanticIconProps): import("react").JSX.Element;
