import type { components } from "../../api/generated/openapi";

export type RunnerID = components["schemas"]["RunnerID"];
export type RunnerSelectionSource =
  components["schemas"]["RunnerSelectionSource"];

export type RunnerOptionalCapability =
  | "image_input"
  | "session_resume"
  | "structured_output"
  | "working_directory"
  | "worktree";

export interface RunnerCapabilitySupport {
  capability: RunnerOptionalCapability;
  detail?: string;
  status: "supported" | "unsupported";
}

export interface RunnerMetadata {
  displayName: string;
  id: RunnerID;
  optionalCapabilities: RunnerCapabilitySupport[];
}

const BUILT_IN_RUNNER_METADATA: Record<RunnerID, RunnerMetadata> = {
  codex: {
    displayName: "Codex",
    id: "codex",
    optionalCapabilities: [
      { capability: "image_input", status: "supported" },
      { capability: "session_resume", status: "supported" },
      { capability: "structured_output", status: "supported" },
      { capability: "working_directory", status: "supported" },
      {
        capability: "worktree",
        detail: "Codex ignores workstation worktree selection in v1.",
        status: "unsupported",
      },
    ],
  },
  gemini: {
    displayName: "Gemini",
    id: "gemini",
    optionalCapabilities: [
      { capability: "image_input", status: "unsupported" },
      { capability: "session_resume", status: "unsupported" },
      { capability: "structured_output", status: "unsupported" },
      { capability: "working_directory", status: "unsupported" },
      { capability: "worktree", status: "unsupported" },
    ],
  },
  kiro: {
    displayName: "Kiro",
    id: "kiro",
    optionalCapabilities: [
      { capability: "image_input", status: "unsupported" },
      { capability: "session_resume", status: "supported" },
      { capability: "structured_output", status: "unsupported" },
      { capability: "working_directory", status: "unsupported" },
      { capability: "worktree", status: "unsupported" },
    ],
  },
  "cursor-cli": {
    displayName: "Cursor CLI",
    id: "cursor-cli",
    optionalCapabilities: [
      { capability: "image_input", status: "unsupported" },
      { capability: "session_resume", status: "supported" },
      { capability: "structured_output", status: "unsupported" },
      { capability: "working_directory", status: "supported" },
      { capability: "worktree", status: "unsupported" },
    ],
  },
  opencode: {
    displayName: "OpenCode",
    id: "opencode",
    optionalCapabilities: [
      { capability: "image_input", status: "unsupported" },
      { capability: "session_resume", status: "supported" },
      { capability: "structured_output", status: "unsupported" },
      { capability: "working_directory", status: "supported" },
      { capability: "worktree", status: "unsupported" },
    ],
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
