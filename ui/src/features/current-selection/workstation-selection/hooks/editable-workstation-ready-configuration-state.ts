import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { EditableWorkstationBehavior } from "../../../current-factory-definition/lib/workstation-behavior";
import {
  applyEditableWorkstationDraft,
  type EditableWorkstationDraft,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { resolveEditableWorkstationValues } from "../../../current-factory-definition/lib/workstation-editable-values";
import { editableWorkstationDraftsEqual } from "../../../current-factory-definition/lib/workstation-guards";
import { resolveEditableWorkstationOverwriteFields } from "../editing/editable-workstation-overwrite-fields";
import type {
  EditableWorkstationPromptHelpState,
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
  EditableWorkstationWorkerOptionsState,
  EditableWorkstationWorkstationOptionsState,
} from "../lib/detail-card-types";
import type { WorkstationDetailMessages } from "../messages/workstation-detail";
import type { RunnerID } from "../editing/runner-metadata";
import { hasEditableWorkstationValidationErrors } from "./editable-workstation-draft-validation";

interface EditableWorkstationSessionState {
  draft: EditableWorkstationDraft;
  latestDefinitionDraft: EditableWorkstationDraft;
  selectionKey: string;
  sessionStartDraft: EditableWorkstationDraft;
}

function resolveWorkerOptionsState(
  draft: EditableWorkstationDraft,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationEmpty"
    | "editableConfigurationWorkerMissing"
    | "editableConfigurationWorkerOptionsEmpty"
  >,
): EditableWorkstationWorkerOptionsState {
  if (!selectedEditableValues) {
    return {
      message: messages.editableConfigurationEmpty,
      status: "error",
    };
  }

  if (selectedEditableValues.workerOptions.length === 0) {
    return {
      message: messages.editableConfigurationWorkerOptionsEmpty,
      status: "empty",
    };
  }

  if (!selectedEditableValues.workerOptions.includes(draft.workerName)) {
    return {
      message: messages.editableConfigurationWorkerMissing,
      status: "error",
    };
  }

  return {
    options: selectedEditableValues.workerOptions,
    status: "ready",
  };
}

function resolveWorkstationOptionsState(
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationEmpty"
    | "editableConfigurationWorkstationOptionsEmpty"
  >,
): EditableWorkstationWorkstationOptionsState {
  if (!selectedEditableValues) {
    return {
      message: messages.editableConfigurationEmpty,
      status: "error",
    };
  }

  if (selectedEditableValues.workstationOptions.length === 0) {
    return {
      message: messages.editableConfigurationWorkstationOptionsEmpty,
      status: "empty",
    };
  }

  return {
    options: selectedEditableValues.workstationOptions,
    status: "ready",
  };
}

function createEditableWorkstationDraftHandlers(
  setSessionState: (
    updater: (
      currentState: EditableWorkstationSessionState | null,
    ) => EditableWorkstationSessionState | null,
  ) => void,
) {
  const updateDraft = (
    updater: (draft: EditableWorkstationDraft) => EditableWorkstationDraft,
  ) => {
    setSessionState((currentState) =>
      currentState
        ? {
            ...currentState,
            draft: updater(currentState.draft),
          }
        : currentState,
    );
  };

  return {
    onPromptChange: (value: string) => {
      updateDraft((draft) => ({ ...draft, prompt: value }));
    },
    onBehaviorChange: (value: EditableWorkstationBehavior) => {
      updateDraft((draft) => ({ ...draft, behavior: value }));
    },
    onGuardsChange: (guards: EditableWorkstationDraft["guards"]) => {
      updateDraft((draft) => ({ ...draft, guards }));
    },
    onInputsChange: (inputs: EditableWorkstationDraft["inputs"]) => {
      updateDraft((draft) => ({ ...draft, inputs }));
    },
    onRunnerChange: (value: RunnerID | null) => {
      updateDraft((draft) => ({ ...draft, runnerName: value }));
    },
    onWorkerChange: (value: string) => {
      updateDraft((draft) => ({ ...draft, workerName: value }));
    },
  };
}

export function buildReadyEditableWorkstationConfigurationState({
  editableDefinition,
  messages,
  promptHelpState,
  promptValidationState,
  resolvedValidationErrors,
  selectedEditableValues,
  selectedNode,
  sessionState,
  setSessionState,
}: {
  editableDefinition: NonNullable<
    ReturnType<typeof useCurrentFactoryDocument>["data"]
  >;
  messages: WorkstationDetailMessages;
  promptHelpState: EditableWorkstationPromptHelpState;
  promptValidationState: EditableWorkstationPromptValidationState;
  resolvedValidationErrors: EditableWorkstationValidationErrors;
  selectedEditableValues: NonNullable<
    ReturnType<typeof resolveEditableWorkstationValues>
  >;
  selectedNode: DashboardWorkstationNode;
  sessionState: EditableWorkstationSessionState;
  setSessionState: (
    updater: (
      currentState: EditableWorkstationSessionState | null,
    ) => EditableWorkstationSessionState | null,
  ) => void;
}) {
  const pendingFactoryDefinition = hasEditableWorkstationValidationErrors(
    resolvedValidationErrors,
  )
    ? null
    : applyEditableWorkstationDraft(
        editableDefinition,
        selectedNode,
        sessionState.draft,
      );
  const draftHandlers = createEditableWorkstationDraftHandlers(setSessionState);

  return {
    baseVersion: editableDefinition.version,
    draft: sessionState.draft,
    hasValidationErrors: hasEditableWorkstationValidationErrors(
      resolvedValidationErrors,
    ),
    initialValues: selectedEditableValues,
    isDirty: !editableWorkstationDraftsEqual(
      sessionState.draft,
      sessionState.sessionStartDraft,
    ),
    markChangesSaved: () => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              latestDefinitionDraft: currentState.draft,
              sessionStartDraft: currentState.draft,
            }
          : currentState,
      );
    },
    onResetToLatest: () => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: currentState.latestDefinitionDraft,
              sessionStartDraft: currentState.latestDefinitionDraft,
            }
          : currentState,
      );
    },
    ...draftHandlers,
    overwriteFieldNames: resolveEditableWorkstationOverwriteFields(
      sessionState.sessionStartDraft,
      sessionState.draft,
      sessionState.latestDefinitionDraft,
    ),
    pendingFactoryDefinition,
    promptDiagnostics:
      promptValidationState.status === "ready"
        ? promptValidationState.diagnostics
        : [],
    promptHelpState,
    promptValidationState,
    status: "ready" as const,
    validationErrors: resolvedValidationErrors,
    workerOptionsState: resolveWorkerOptionsState(
      sessionState.draft,
      selectedEditableValues,
      messages,
    ),
    workstationOptionsState: resolveWorkstationOptionsState(
      selectedEditableValues,
      messages,
    ),
  };
}
