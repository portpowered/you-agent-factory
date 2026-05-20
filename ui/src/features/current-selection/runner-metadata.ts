import type { components } from "../../api/generated/openapi";

export type RunnerID = components["schemas"]["RunnerID"];
export type RunnerBaselineCapability =
  components["schemas"]["FactoryWorldRunnerBaselineCapability"];
export type RunnerOptionalCapability =
  components["schemas"]["FactoryWorldRunnerOptionalCapability"];
export type RunnerOptionalCapabilityStatus =
  components["schemas"]["FactoryWorldRunnerOptionalCapabilityStatus"];

export interface RunnerCapabilitySupport {
  capability: RunnerOptionalCapability;
  detail?: string;
  status: RunnerOptionalCapabilityStatus;
}

export interface RunnerCapabilitiesMetadata {
  baselineCapabilities: RunnerBaselineCapability[];
  optionalCapabilities: RunnerCapabilitySupport[];
}

export interface RunnerMetadata {
  capabilities: RunnerCapabilitiesMetadata;
  displayName: string;
  id: RunnerID;
}

const V1_BASELINE_CAPABILITIES: RunnerBaselineCapability[] = [
  "prompt_submission",
  "tool_execution",
];

const BUILT_IN_RUNNER_METADATA: Record<RunnerID, RunnerMetadata> = {
  codex: {
    capabilities: {
      baselineCapabilities: V1_BASELINE_CAPABILITIES,
      optionalCapabilities: [
        { capability: "image_input", status: "supported" },
        { capability: "session_resume", status: "supported" },
        { capability: "structured_output", status: "supported" },
        { capability: "working_directory", status: "supported" },
        {
          capability: "worktree",
          detail: "Codex rejects workstation worktree selection in v1.",
          status: "unsupported",
        },
      ],
    },
    displayName: "Codex",
    id: "codex",
  },
  gemini: {
    capabilities: {
      baselineCapabilities: V1_BASELINE_CAPABILITIES,
      optionalCapabilities: [
        { capability: "image_input", status: "unsupported" },
        { capability: "session_resume", status: "unsupported" },
        { capability: "structured_output", status: "unsupported" },
        { capability: "working_directory", status: "unsupported" },
        { capability: "worktree", status: "unsupported" },
      ],
    },
    displayName: "Gemini",
    id: "gemini",
  },
  kiro: {
    capabilities: {
      baselineCapabilities: V1_BASELINE_CAPABILITIES,
      optionalCapabilities: [
        { capability: "image_input", status: "unsupported" },
        { capability: "session_resume", status: "supported" },
        { capability: "structured_output", status: "unsupported" },
        { capability: "working_directory", status: "unsupported" },
        { capability: "worktree", status: "unsupported" },
      ],
    },
    displayName: "Kiro",
    id: "kiro",
  },
  "cursor-cli": {
    capabilities: {
      baselineCapabilities: V1_BASELINE_CAPABILITIES,
      optionalCapabilities: [
        { capability: "image_input", status: "unsupported" },
        { capability: "session_resume", status: "supported" },
        { capability: "structured_output", status: "unsupported" },
        { capability: "working_directory", status: "supported" },
        { capability: "worktree", status: "unsupported" },
      ],
    },
    displayName: "Cursor CLI",
    id: "cursor-cli",
  },
  opencode: {
    capabilities: {
      baselineCapabilities: V1_BASELINE_CAPABILITIES,
      optionalCapabilities: [
        { capability: "image_input", status: "unsupported" },
        { capability: "session_resume", status: "supported" },
        { capability: "structured_output", status: "unsupported" },
        { capability: "working_directory", status: "supported" },
        { capability: "worktree", status: "unsupported" },
      ],
    },
    displayName: "OpenCode",
    id: "opencode",
  },
};

export const BUILT_IN_RUNNER_IDS = Object.keys(
  BUILT_IN_RUNNER_METADATA,
) as RunnerID[];

export function getRunnerMetadata(
  runnerID: string | null | undefined,
): RunnerMetadata | null {
  if (!runnerID) {
    return null;
  }

  return BUILT_IN_RUNNER_METADATA[runnerID as RunnerID] ?? null;
}

export function getRunnerDisplayName(
  runnerID: string | null | undefined,
): string | null {
  return getRunnerMetadata(runnerID)?.displayName ?? null;
}

export function resolveSelectedRunnerMetadata(
  runner:
    | {
        capabilities?: Partial<RunnerCapabilitiesMetadata>;
        displayName?: string;
        runnerId?: RunnerID;
      }
    | null
    | undefined,
): RunnerMetadata | null {
  if (!runner?.runnerId) {
    return null;
  }

  const fallbackMetadata = getRunnerMetadata(runner.runnerId);
  return {
    capabilities: {
      baselineCapabilities:
        runner.capabilities?.baselineCapabilities ??
        fallbackMetadata?.capabilities.baselineCapabilities ??
        V1_BASELINE_CAPABILITIES,
      optionalCapabilities:
        runner.capabilities?.optionalCapabilities ??
        fallbackMetadata?.capabilities.optionalCapabilities ??
        [],
    },
    displayName:
      runner.displayName ?? fallbackMetadata?.displayName ?? runner.runnerId,
    id: runner.runnerId,
  };
}
