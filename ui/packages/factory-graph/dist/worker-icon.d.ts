import type { GraphSemanticIconKind } from "./semantic-icon.js";
import type { FactoryGraphVisualState } from "./visual-state.js";
export type FactoryGraphWorkerIconKind = Extract<GraphSemanticIconKind, "worker" | "script" | "codex" | "claude" | "gemini" | "antigravity">;
export type FactoryGraphWorkerProviderKind = Exclude<FactoryGraphWorkerIconKind, "worker" | "script">;
export declare const FACTORY_GRAPH_WORKER_TYPES: readonly ["INFERENCE_WORKER", "AGENT_WORKER", "SCRIPT_WORKER", "POLLER_WORKER", "MODEL_WORKER", "HOSTED_WORKER"];
export type FactoryGraphWorkerType = (typeof FACTORY_GRAPH_WORKER_TYPES)[number];
/** Selects a worker glyph from projected, canonical worker metadata. */
export declare function factoryGraphWorkerIconKind(workerType: string | null | undefined, runnerId: string | null | undefined): FactoryGraphWorkerIconKind;
/** Resolves only the explicitly supported provider spellings, never substrings. */
export declare function factoryGraphWorkerProviderKind(providerId: string | null | undefined): FactoryGraphWorkerProviderKind | undefined;
export declare function factoryGraphWorkerProviderLabel(providerKind: FactoryGraphWorkerProviderKind | undefined): string | undefined;
export declare function isFactoryGraphKnownWorkerType(workerType: string | null | undefined): workerType is FactoryGraphWorkerType;
/** Returns an unfamiliar worker kind unchanged for a neutral raw-value label. */
export declare function factoryGraphUnknownWorkerType(workerType: string | null | undefined): string | undefined;
/**
 * Temporary worker-owned seam for the parent node's resolved accent.
 *
 * gfxux-2 is adding a shared nested-accent contract. Keeping this call behind
 * the worker icon module makes that contract directly swappable without
 * changing the shared visual-state or node-style modules in this lane.
 */
export declare function factoryGraphWorkerIconClassName(visualState: FactoryGraphVisualState, fallbackClassName?: string): string;
