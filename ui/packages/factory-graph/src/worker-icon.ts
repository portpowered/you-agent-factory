import type { GraphSemanticIconKind } from "./semantic-icon.js";

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

function normalize(value: string | null | undefined): string {
  return typeof value === "string" ? value.trim().toUpperCase() : "";
}
