import type { ReactNode } from "react";

import { factoryGraphNodeWrappedTextClassName } from "./semantic-node-style.js";

export type FactoryGraphNodeInteractionBadgeTone =
  | "danger"
  | "info"
  | "neutral"
  | "success"
  | "warning";

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

export function FactoryGraphNodeInteractionOverlayView({
  overlay,
}: {
  overlay?: FactoryGraphNodeInteractionOverlay;
}) {
  if (!overlay) return null;

  const badges = overlay.badges ?? [];
  if (badges.length === 0 && !overlay.connectionHint) return null;

  return (
    <div className="grid min-w-0 gap-1" data-graph-interaction-overlay>
      {badges.length > 0 ? (
        <div className="flex min-w-0 flex-wrap items-center gap-1">
          {badges.map((badge) => (
            <span
              className={badgeClassName(badge.tone)}
              data-graph-interaction-badge
              key={`${badge.tone ?? "neutral"}-${String(badge.label)}`}
              role="status"
            >
              {badge.label}
            </span>
          ))}
        </div>
      ) : null}
      {overlay.connectionHint ? (
        <p
          className={factoryGraphNodeWrappedTextClassName(
            "m-0 text-[0.65rem] leading-5 text-on-surface-subtle",
          )}
          data-graph-interaction-hint
        >
          {overlay.connectionHint}
        </p>
      ) : null}
    </div>
  );
}

function badgeClassName(
  tone: FactoryGraphNodeInteractionBadgeTone = "neutral",
): string {
  const toneClassName = {
    danger:
      "border-af-danger-border bg-error-container text-on-error-container",
    info: "border-af-info-border bg-info-container text-on-info-container",
    neutral: "border-outline bg-surface-container-low text-on-surface-variant",
    success:
      "border-af-success-border bg-success-container text-on-success-container",
    warning:
      "border-af-warning-border bg-warning-container text-on-warning-container",
  }[tone];
  return [
    "inline-flex min-h-6 w-fit items-center justify-center rounded-full border px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
    toneClassName,
  ].join(" ");
}
