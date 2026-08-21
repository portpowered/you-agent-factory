import type { GraphSemanticIconKind } from "./semantic-icon.js";
import { factoryGraphNodeVisualIconClassName } from "./semantic-node-style.js";
import type { FactoryGraphVisualState } from "./visual-state.js";

export type FactoryGraphWorkerIconKind = Extract<
  GraphSemanticIconKind,
  "worker" | "script" | "codex" | "claude" | "antigravity"
>;

export const FACTORY_GRAPH_WORKER_TYPES = [
  "INFERENCE_WORKER",
  "AGENT_WORKER",
  "SCRIPT_WORKER",
  "POLLER_WORKER",
  "MODEL_WORKER",
  "HOSTED_WORKER",
] as const;

export type FactoryGraphWorkerType =
  (typeof FACTORY_GRAPH_WORKER_TYPES)[number];

/** Selects a worker glyph from projected, canonical worker metadata. */
export function factoryGraphWorkerIconKind(
  workerType: string | null | undefined,
  runnerId: string | null | undefined,
): FactoryGraphWorkerIconKind {
  if (workerType === "SCRIPT_WORKER") {
    return "script";
  }

  // Canonical worker kinds are exact values. A future kind must not inherit a
  // runner-specific glyph just because its spelling resembles a known kind.
  if (factoryGraphUnknownWorkerType(workerType) !== undefined) {
    return "worker";
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

export function isFactoryGraphKnownWorkerType(
  workerType: string | null | undefined,
): workerType is FactoryGraphWorkerType {
  return (
    typeof workerType === "string" &&
    FACTORY_GRAPH_WORKER_TYPES.includes(workerType as FactoryGraphWorkerType)
  );
}

/** Returns an unfamiliar worker kind unchanged for a neutral raw-value label. */
export function factoryGraphUnknownWorkerType(
  workerType: string | null | undefined,
): string | undefined {
  return typeof workerType === "string" &&
    workerType.length > 0 &&
    !isFactoryGraphKnownWorkerType(workerType)
    ? workerType
    : undefined;
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
  fallbackClassName = "text-info",
): string {
  return factoryGraphNodeVisualIconClassName(visualState, fallbackClassName);
}

function normalize(value: string | null | undefined): string {
  return typeof value === "string" ? value.trim().toUpperCase() : "";
}
