import { factoryGraphNodeVisualIconClassName } from "./semantic-node-style.js";
const FACTORY_GRAPH_WORKER_PROVIDER_ALIASES = {
    AGY: "antigravity",
    ANTHROPIC: "claude",
    ANTIGRAVITY: "antigravity",
    CLAUDE: "claude",
    "CLAUDE CLI": "claude",
    CLAUDE_CLI: "claude",
    "CLAUDE-CLI": "claude",
    CODEX: "codex",
    GEMINI: "gemini",
    "LOCAL CLAUDE": "claude",
    LOCAL_CLAUDE: "claude",
    "LOCAL-CLAUDE": "claude",
    OPENAI: "codex",
    OPENAI_CODEX: "codex",
    "OPENAI-CODEX": "codex",
};
export const FACTORY_GRAPH_WORKER_TYPES = [
    "INFERENCE_WORKER",
    "AGENT_WORKER",
    "SCRIPT_WORKER",
    "POLLER_WORKER",
    "MODEL_WORKER",
    "HOSTED_WORKER",
];
/** Selects a worker glyph from projected, canonical worker metadata. */
export function factoryGraphWorkerIconKind(workerType, runnerId) {
    if (workerType === "SCRIPT_WORKER") {
        return "script";
    }
    // Canonical worker kinds are exact values. A future kind must not inherit a
    // runner-specific glyph just because its spelling resembles a known kind.
    if (factoryGraphUnknownWorkerType(workerType) !== undefined) {
        return "worker";
    }
    return factoryGraphWorkerProviderKind(runnerId) ?? "worker";
}
/** Resolves only the explicitly supported provider spellings, never substrings. */
export function factoryGraphWorkerProviderKind(providerId) {
    return FACTORY_GRAPH_WORKER_PROVIDER_ALIASES[normalize(providerId)];
}
export function factoryGraphWorkerProviderLabel(providerKind) {
    switch (providerKind) {
        case "antigravity":
            return "Antigravity";
        case "claude":
            return "Claude/Anthropic";
        case "codex":
            return "Codex/OpenAI";
        case "gemini":
            return "Gemini";
        default:
            return undefined;
    }
}
export function isFactoryGraphKnownWorkerType(workerType) {
    return (typeof workerType === "string" &&
        FACTORY_GRAPH_WORKER_TYPES.includes(workerType));
}
/** Returns an unfamiliar worker kind unchanged for a neutral raw-value label. */
export function factoryGraphUnknownWorkerType(workerType) {
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
export function factoryGraphWorkerIconClassName(visualState, fallbackClassName = "text-info") {
    return factoryGraphNodeVisualIconClassName(visualState, fallbackClassName);
}
function normalize(value) {
    return typeof value === "string" ? value.trim().toUpperCase() : "";
}
