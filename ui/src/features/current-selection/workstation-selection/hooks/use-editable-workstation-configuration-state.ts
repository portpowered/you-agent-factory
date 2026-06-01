import { useEffect, useMemo, useState } from "react";

import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import {
  type EditableWorkstationBehavior,
  workstationBehaviorRequiresPrompt,
} from "../../../current-factory-definition/lib/workstation-behavior";
import {
  applyEditableWorkstationDraft,
  type EditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { workstationRequiresWorkerAssignment } from "../../../current-factory-definition/lib/workstation-worker-assignment";
import type { DashboardSelection } from "../../base/state/selection-types";
import { resolveEditableWorkstationOverwriteFields } from "../editing/editable-workstation-overwrite-fields";
import {
  resolvePromptHelpState,
  resolvePromptValidationState,
} from "../editing/editable-workstation-prompt-state";
import type { RunnerID } from "../editing/runner-metadata";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationPromptHelpState,
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
} from "../lib/detail-card-types";
import {
  hasEditableWorkstationValidationErrors,
  resolveWorkerOptionsState,
  validateEditableWorkstationDraft,
} from "../lib/editable-workstation-configuration-validation";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../messages/workstation-detail";
import { useCurrentWorkstationPromptTemplateContract } from "./useCurrentWorkstationPromptTemplateContract";
import { useCurrentWorkstationPromptTemplateValidation } from "./useCurrentWorkstationPromptTemplateValidation";

interface EditableWorkstationSessionState {
  draft: EditableWorkstationDraft;
  latestDefinitionDraft: EditableWorkstationDraft;
  selectionKey: string;
  sessionStartDraft: EditableWorkstationDraft;
}

export function useEditableWorkstationConfigurationState(
  selection: DashboardSelection | null,
  selectedNode: DashboardWorkstationNode | null,
  locale?: string | null,
): EditableWorkstationConfigurationState | undefined {
  const isNodeSelection = selection?.kind === "node" && selectedNode != null;
  const messages = getWorkstationDetailMessages(locale);
  const editableDefinition = useCurrentFactoryDocument(isNodeSelection);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkstationSession(
      editableDefinition.data,
      selectedNode,
      selection,
    );
  const promptTemplateContract = useCurrentWorkstationPromptTemplateContract(
    selectedEditableValues?.workstationName,
    isNodeSelection && selectedEditableValues != null,
  );
  const shouldValidatePrompt =
    isNodeSelection &&
    sessionState != null &&
    selectedEditableValues != null &&
    workstationRequiresWorkerAssignment({
      type: selectedEditableValues.workstationType,
    }) &&
    workstationBehaviorRequiresPrompt(sessionState.draft.behavior);
  const promptValidation = useCurrentWorkstationPromptTemplateValidation(
    selectedEditableValues?.workstationName,
    sessionState?.draft.prompt,
    shouldValidatePrompt && selectedEditableValues != null,
  );

  if (!isNodeSelection) {
    return undefined;
  }

  if (editableDefinition.isPending) {
    return { status: "loading" };
  }

  if (editableDefinition.isError) {
    return {
      errorMessage: editableDefinition.error.message,
      status: "error",
    };
  }

  if (!editableDefinition.data || !selectedEditableValues || !sessionState) {
    return {
      message: messages.editableConfigurationEmpty,
      status: "empty",
    };
  }

  const promptHelpState = resolvePromptHelpState(
    promptTemplateContract,
    messages,
  );
  const promptValidationState = resolvePromptValidationState(
    promptValidation,
    sessionState.draft.prompt,
    messages,
  );

  const resolvedValidationErrors = validateEditableWorkstationDraft(
    sessionState.draft,
    selectedEditableValues,
    promptValidationState,
    messages,
  );

  return buildReadyEditableWorkstationConfigurationState({
    editableDefinition: editableDefinition.data,
    messages,
    promptHelpState,
    promptValidationState,
    resolvedValidationErrors,
    selectedEditableValues,
    selectedNode,
    sessionState,
    setSessionState,
  });
}

function useEditableWorkstationSession(
  editableDefinition: ReturnType<typeof useCurrentFactoryDocument>["data"],
  selectedNode: DashboardWorkstationNode | null,
  selection: DashboardSelection | null,
) {
  const selectedEditableValues = useMemo(() => {
    if (selection?.kind !== "node" || !selectedNode || !editableDefinition) {
      return null;
    }

    return resolveEditableWorkstationValues(editableDefinition, selectedNode);
  }, [editableDefinition, selectedNode, selection]);
  const selectionKey =
    selection?.kind === "node" && selectedNode
      ? `${selectedNode.node_id}:${selectedNode.transition_id}:${selectedNode.workstation_name}`
      : null;
  const [sessionState, setSessionState] =
    useState<EditableWorkstationSessionState | null>(null);

  useEffect(() => {
    setSessionState((currentState) =>
      syncEditableWorkstationSession(
        currentState,
        selectedEditableValues,
        selectionKey,
      ),
    );
  }, [selectedEditableValues, selectionKey]);

  return {
    selectedEditableValues,
    sessionState,
    setSessionState,
  };
}

export {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "../lib/editable-workstation-configuration-validation";

function areEditableDraftsEqual(
  left: EditableWorkstationDraft,
  right: EditableWorkstationDraft,
): boolean {
  return (
    left.behavior === right.behavior &&
    left.prompt === right.prompt &&
    left.runnerName === right.runnerName &&
    left.workerName === right.workerName
  );
}

function buildReadyEditableWorkstationConfigurationState({
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

  return {
    baseVersion: editableDefinition.version,
    draft: sessionState.draft,
    hasValidationErrors: hasEditableWorkstationValidationErrors(
      resolvedValidationErrors,
    ),
    initialValues: selectedEditableValues,
    isDirty: !areEditableDraftsEqual(
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
    onPromptChange: (value: string) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                prompt: value,
              },
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
    onBehaviorChange: (value: EditableWorkstationBehavior) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                behavior: value,
              },
            }
          : currentState,
      );
    },
    onRunnerChange: (value: RunnerID | null) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                runnerName: value,
              },
            }
          : currentState,
      );
    },
    onWorkerChange: (value: string) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                workerName: value,
              },
            }
          : currentState,
      );
    },
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
  };
}

function syncEditableWorkstationSession(
  currentState: EditableWorkstationSessionState | null,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkstationValues>,
  selectionKey: string | null,
): EditableWorkstationSessionState | null {
  if (!selectionKey || !selectedEditableValues) {
    return null;
  }

  const initialDraft = editableWorkstationDraftFromValues(
    selectedEditableValues,
  );
  if (!currentState || currentState.selectionKey !== selectionKey) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  if (
    areEditableDraftsEqual(currentState.draft, currentState.sessionStartDraft)
  ) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  return areEditableDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
