import {
  type ApiRunnerID,
  OPENAPI_RUNNER_IDS,
} from "../messages/runner-openapi-enums";

export interface RunnerMetadata {
  displayName: string;
  id: ApiRunnerID;
}

const BUILT_IN_RUNNER_METADATA: Record<ApiRunnerID, RunnerMetadata> = {
  codex: {
    displayName: "Codex",
    id: "codex",
  },
  claude: {
    displayName: "Claude",
    id: "claude",
  },
  "cursor-cli": {
    displayName: "Cursor CLI",
    id: "cursor-cli",
  },
  antigravity: {
    displayName: "Antigravity",
    id: "antigravity",
  },
};

export const BUILT_IN_RUNNER_IDS: ApiRunnerID[] = [...OPENAPI_RUNNER_IDS];

export function getRunnerMetadata(
  runnerID: string | null | undefined,
): RunnerMetadata | null {
  if (!runnerID) {
    return null;
  }

  return BUILT_IN_RUNNER_METADATA[runnerID as ApiRunnerID] ?? null;
}

export function getRunnerDisplayName(
  runnerID: string | null | undefined,
): string | null {
  return getRunnerMetadata(runnerID)?.displayName ?? null;
}
