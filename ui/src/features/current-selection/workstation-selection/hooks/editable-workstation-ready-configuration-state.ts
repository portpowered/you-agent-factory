import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import type { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import {
  type EditableWorkstationBehavior,
  workstationBehaviorRequiresPrompt,
} from "../../../current-factory-definition/lib/workstation-behavior";
import {
  applyEditableWorkstationDraft,
  type EditableWorkstationDraft,
  type EditableWorkstationValues,
  type resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { editableWorkstationDraftsEqual } from "../../../current-factory-definition/lib/workstation-guards";
import { workstationRequiresWorkerAssignment } from "../../../current-factory-definition/lib/workstation-worker-assignment";
import {
  resolveDraftForBehaviorChange,
  updateEditableWorkstationCronDraft,
} from "../editing/editable-workstation-cron-draft-mutators";
import { resolveEditableWorkstationOverwriteFields } from "../editing/editable-workstation-overwrite-fields";
import type { RunnerID } from "../editing/runner-metadata";
import type {
  EditableWorkstationPromptHelpState,
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
  EditableWorkstationWorkstationOptionsState,
} from "../lib/detail-card-types";
import {
  hasEditableWorkstationValidationErrors,
  resolveWorkerOptionsState,
} from "../lib/editable-workstation-configuration-validation";
import type { WorkstationDetailMessages } from "../messages/workstation-detail";

interface EditableWorkstationSessionState {
  draft: EditableWorkstationDraft;
  latestDefinitionDraft: EditableWorkstationDraft;
  selectionKey: string;
  sessionStartDraft: EditableWorkstationDraft;
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
  selectedEditableValues: EditableWorkstationValues,
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
      updateDraft((draft) =>
        resolveDraftForBehaviorChange(draft, value, selectedEditableValues),
      );
    },
    onCronExpiryWindowChange: (value: string) => {
      updateDraft((draft) =>
        updateEditableWorkstationCronDraft(draft, { expiryWindow: value }),
      );
    },
    onCronJitterChange: (value: string) => {
      updateDraft((draft) =>
        updateEditableWorkstationCronDraft(draft, { jitter: value }),
      );
    },
    onCronScheduleChange: (value: string) => {
      updateDraft((draft) =>
        updateEditableWorkstationCronDraft(draft, { schedule: value }),
      );
    },
    onCronTriggerAtStartChange: (value: boolean) => {
      updateDraft((draft) =>
        updateEditableWorkstationCronDraft(draft, { triggerAtStart: value }),
      );
    },
    onNameChange: (value: string) => {
      updateDraft((draft) => ({ ...draft, name: value }));
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
  const hasValidationErrors = hasEditableWorkstationValidationErrors(
    resolvedValidationErrors,
  );
  const promptValidationBlocksPendingFactory =
    workstationRequiresWorkerAssignment({
      type: selectedEditableValues.workstationType,
    }) &&
    workstationBehaviorRequiresPrompt(sessionState.draft.behavior) &&
    sessionState.draft.prompt.trim().length > 0 &&
    promptValidationState.status === "loading";
  const pendingFactoryDefinition =
    hasValidationErrors || promptValidationBlocksPendingFactory
      ? null
      : applyEditableWorkstationDraft(
          editableDefinition,
          selectedNode,
          sessionState.draft,
        );
  const draftHandlers = createEditableWorkstationDraftHandlers(
    selectedEditableValues,
    setSessionState,
  );

  return {
    baseVersion: editableDefinition.version,
    draft: sessionState.draft,
    hasValidationErrors,
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
    savedFactoryDefinition: editableDefinition,
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
