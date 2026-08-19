import type { GraphSemanticIconKind } from "./semantic-icon.js";
import type { FactoryGraphVisualState } from "./visual-state.js";
export type FactoryGraphWorkerIconKind = Extract<GraphSemanticIconKind, "worker" | "script" | "codex" | "claude" | "antigravity">;
/** Selects a worker glyph from projected, canonical worker metadata. */
export declare function factoryGraphWorkerIconKind(workerType: string | null | undefined, runnerId: string | null | undefined): FactoryGraphWorkerIconKind;
/**
 * Temporary worker-owned seam for the parent node's resolved accent.
 *
 * gfxux-2 is adding a shared nested-accent contract. Keeping this call behind
 * the worker icon module makes that contract directly swappable without
 * changing the shared visual-state or node-style modules in this lane.
 */
export declare function factoryGraphWorkerIconClassName(visualState: FactoryGraphVisualState): string;
