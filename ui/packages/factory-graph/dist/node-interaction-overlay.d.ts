import type { ReactNode } from "react";
export type FactoryGraphNodeInteractionBadgeTone = "danger" | "info" | "neutral" | "success" | "warning";
export interface FactoryGraphNodeInteractionBadge {
    label: ReactNode;
    tone?: FactoryGraphNodeInteractionBadgeTone;
}
/**
 * Typed host-owned interaction and runtime feedback for a semantic node.
 *
 * The package owns how this feedback is placed and styled. Hosts only supply
 * their operation state; they do not replace the semantic node renderer.
 */
export interface FactoryGraphNodeInteractionOverlay {
    badges?: readonly FactoryGraphNodeInteractionBadge[];
    connectionHint?: string;
    draftStatus?: "addition" | "none" | "removal";
}
export declare function FactoryGraphNodeInteractionOverlayView({ overlay, }: {
    overlay?: FactoryGraphNodeInteractionOverlay;
}): import("react/jsx-runtime").JSX.Element | null;
