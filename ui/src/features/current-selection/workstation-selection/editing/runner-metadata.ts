import type { components } from "../../../../api/generated/openapi";
import { OPENAPI_RUNNER_IDS } from "../messages/runner-openapi-enums";

export type RunnerID = components["schemas"]["RunnerID"];

export interface RunnerMetadata {
  displayName: string;
  id: RunnerID;
}

const BUILT_IN_RUNNER_METADATA: Record<RunnerID, RunnerMetadata> = {
  codex: {
    displayName: "Codex",
    id: "codex",
  },
  gemini: {
    displayName: "Gemini",
    id: "gemini",
  },
  kiro: {
    displayName: "Kiro",
    id: "kiro",
  },
  "cursor-cli": {
    displayName: "Cursor CLI",
    id: "cursor-cli",
  },
  opencode: {
    displayName: "OpenCode",
    id: "opencode",
  },
};

export const BUILT_IN_RUNNER_IDS: RunnerID[] = [...OPENAPI_RUNNER_IDS];

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
