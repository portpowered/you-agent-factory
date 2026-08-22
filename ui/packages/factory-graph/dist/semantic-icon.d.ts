export declare const GRAPH_SEMANTIC_ICON_KINDS: readonly ["queue", "processing", "terminal", "failed", "resource", "worker", "script", "codex", "claude", "antigravity", "work-type", "constraint", "doc", "limit", "workstation", "repeater", "cron", "poller", "active-work"];
export type GraphSemanticIconKind = (typeof GRAPH_SEMANTIC_ICON_KINDS)[number];
export interface GraphSemanticIconProps {
    className?: string;
    kind: GraphSemanticIconKind | (string & {});
    label?: string;
    locale?: string;
}
export declare function graphSemanticIconLabel(kind: GraphSemanticIconProps["kind"]): string;
export declare function GraphSemanticIcon({ className, kind, label, }: GraphSemanticIconProps): import("react/jsx-runtime").JSX.Element;
