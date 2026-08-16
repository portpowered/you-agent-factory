import type { GraphSemanticIconKind } from "./semantic-icon.js";
import { factoryGraphNodeVisualIconClassName } from "./semantic-node-style.js";
import type { FactoryGraphVisualState } from "./visual-state.js";

export type FactoryGraphWorkerIconKind = Extract<
  GraphSemanticIconKind,
  "worker" | "script" | "codex" | "claude" | "antigravity"
>;

/** Selects a worker glyph from projected, canonical worker metadata. */
export function factoryGraphWorkerIconKind(
  workerType: string | null | undefined,
  runnerId: string | null | undefined,
): FactoryGraphWorkerIconKind {
  if (normalize(workerType) === "SCRIPT_WORKER") {
    return "script";
  }

  switch (normalize(runnerId)) {
    case "CODEX":
      return "codex";
    case "CLAUDE":
      return "claude";
    case "ANTIGRAVITY":
      return "antigravity";
    default:
      return "worker";
  }
}

/**
 * Temporary worker-owned seam for the parent node's resolved accent.
 *
 * gfxux-2 is adding a shared nested-accent contract. Keeping this call behind
 * the worker icon module makes that contract directly swappable without
 * changing the shared visual-state or node-style modules in this lane.
 */
export function factoryGraphWorkerIconClassName(
  visualState: FactoryGraphVisualState,
): string {
  return factoryGraphNodeVisualIconClassName(visualState, "text-info");
}

function normalize(value: string | null | undefined): string {
  return typeof value === "string" ? value.trim().toUpperCase() : "";
}
