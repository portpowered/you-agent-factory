import {
  type ApiModelProviderSelection,
  OPENAPI_MODEL_PROVIDER_SELECTIONS,
} from "../messages/runner-openapi-enums";

export interface RunnerMetadata {
  displayName: string;
  id: ApiModelProviderSelection;
}

const BUILT_IN_MODEL_PROVIDER_METADATA: Partial<
  Record<ApiModelProviderSelection, RunnerMetadata>
> = {
  CLAUDE: {
    displayName: "Claude",
    id: "CLAUDE",
  },
  CODEX: {
    displayName: "Codex",
    id: "CODEX",
  },
  GEMINI: {
    displayName: "Gemini",
    id: "GEMINI",
  },
  KIRO: {
    displayName: "Kiro",
    id: "KIRO",
  },
  CURSOR: {
    displayName: "Cursor CLI",
    id: "CURSOR",
  },
  OPENCODE: {
    displayName: "OpenCode",
    id: "OPENCODE",
  },
};

export const BUILT_IN_RUNNER_IDS: ApiModelProviderSelection[] = [
  ...OPENAPI_MODEL_PROVIDER_SELECTIONS,
].filter((value) => value !== "DEFAULT");

export function getRunnerMetadata(
  modelProvider: string | null | undefined,
): RunnerMetadata | null {
  if (!modelProvider) {
    return null;
  }

  const normalized = modelProvider.trim().toUpperCase();
  return BUILT_IN_MODEL_PROVIDER_METADATA[normalized as ApiModelProviderSelection] ?? null;
}

export function getRunnerDisplayName(
  modelProvider: string | null | undefined,
): string | null {
  return getRunnerMetadata(modelProvider)?.displayName ?? null;
}
