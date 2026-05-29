import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";
import type {
  EditableWorkstationPromptDiagnostic,
  EditableWorkstationPromptHelpState,
  EditableWorkstationPromptValidationState,
} from "../lib/detail-card-types";
import type { useCurrentWorkstationPromptTemplateContract } from "../hooks/useCurrentWorkstationPromptTemplateContract";
import type { useCurrentWorkstationPromptTemplateValidation } from "../hooks/useCurrentWorkstationPromptTemplateValidation";

export function resolvePromptHelpState(
  promptTemplateContract: ReturnType<
    typeof useCurrentWorkstationPromptTemplateContract
  >,
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationPromptHelpEmpty"
    | "editableConfigurationPromptHelpFallbackError"
  >,
): EditableWorkstationPromptHelpState {
  if (promptTemplateContract.isPending) {
    return { status: "loading" };
  }

  if (promptTemplateContract.isError) {
    return {
      errorMessage:
        promptTemplateContract.error.message ||
        messages.editableConfigurationPromptHelpFallbackError,
      status: "error",
    };
  }

  if (!promptTemplateContract.data) {
    return {
      message: messages.editableConfigurationPromptHelpEmpty,
      status: "empty",
    };
  }

  if (
    promptTemplateContract.data.availableVariables.length === 0 &&
    promptTemplateContract.data.unavailableAccessPatterns.length === 0
  ) {
    return {
      message: messages.editableConfigurationPromptHelpEmpty,
      status: "empty",
    };
  }

  return {
    contract: promptTemplateContract.data,
    status: "ready",
  };
}

export function resolvePromptValidationState(
  promptValidation: ReturnType<
    typeof useCurrentWorkstationPromptTemplateValidation
  >,
  prompt: string,
  messages: Pick<
    WorkstationDetailMessages,
    "editableConfigurationPromptValidationFallbackError"
  >,
): EditableWorkstationPromptValidationState {
  if (prompt.trim().length === 0) {
    return { status: "idle" };
  }

  if (promptValidation.isPending) {
    return { status: "loading" };
  }

  if (promptValidation.isError) {
    return {
      errorMessage:
        promptValidation.error.message ||
        messages.editableConfigurationPromptValidationFallbackError,
      status: "error",
    };
  }

  if (!promptValidation.data) {
    return {
      errorMessage: messages.editableConfigurationPromptValidationFallbackError,
      status: "error",
    };
  }

  return {
    diagnostics: promptValidation.data.diagnostics.map((diagnostic) =>
      editablePromptDiagnosticFromAPI(diagnostic),
    ),
    result: promptValidation.data,
    status: "ready",
  };
}

function editablePromptDiagnosticFromAPI(
  diagnostic: NonNullable<
    ReturnType<
      typeof useCurrentWorkstationPromptTemplateValidation
    >["data"]
  >["diagnostics"][number],
): EditableWorkstationPromptDiagnostic {
  return {
    endOffset: diagnostic.endOffset,
    kind: diagnostic.kind,
    message: diagnostic.message,
    path: diagnostic.path ?? undefined,
    sourceText: diagnostic.sourceText ?? undefined,
    startOffset: diagnostic.startOffset,
  };
}
