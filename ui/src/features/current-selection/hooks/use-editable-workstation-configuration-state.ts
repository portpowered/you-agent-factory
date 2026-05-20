import { useEffect, useMemo, useState } from "react";

import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import { useCurrentEditableFactoryDefinition } from "../../current-factory-definition";
import {
  applyEditableWorkstationDraft,
  type EditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../../current-factory-definition/workstation-editable-values";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationPromptHelpState,
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
  EditableWorkstationWorkerOptionsState,
} from "../detail-card-types";
import {
  resolvePromptHelpState,
  resolvePromptValidationState,
} from "../editable-workstation-prompt-state";
import { resolveEditableWorkstationOverwriteFields } from "../editable-workstation-overwrite-fields";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../messages";
import type { RunnerID } from "../runner-metadata";
import type { DashboardSelection } from "../types";
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
  const editableDefinition =
    useCurrentEditableFactoryDefinition(isNodeSelection);
  const promptTemplateContract = useCurrentWorkstationPromptTemplateContract(
    selectedNode?.workstation_name,
    isNodeSelection,
  );
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkstationSession(
      editableDefinition.data,
      selectedNode,
      selection,
    );
  const promptValidation = useCurrentWorkstationPromptTemplateValidation(
    selectedNode?.workstation_name,
    sessionState?.draft.prompt,
    isNodeSelection,
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
  editableDefinition: ReturnType<
    typeof useCurrentEditableFactoryDefinition
  >["data"],
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

export function validateEditableWorkstationDraft(
  draft: EditableWorkstationDraft,
  selectedEditableValues?: ReturnType<typeof resolveEditableWorkstationValues>,
  promptValidationState: EditableWorkstationPromptValidationState = {
    status: "idle",
  },
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationPromptRequired"
    | "editableConfigurationPromptValidationLoading"
    | "editableConfigurationPromptValidationErrorPrefix"
    | "editableConfigurationPromptDiagnosticsSummary"
    | "editableConfigurationWorkerRequired"
    | "editableConfigurationWorkerUnavailable"
  > = getWorkstationDetailMessages(undefined),
): EditableWorkstationValidationErrors {
  const validationErrors: EditableWorkstationValidationErrors = {};

  if (draft.workerName.trim().length === 0) {
    validationErrors.workerName = messages.editableConfigurationWorkerRequired;
  } else if (
    selectedEditableValues &&
    !selectedEditableValues.workerOptions.includes(draft.workerName)
  ) {
    validationErrors.workerName =
      messages.editableConfigurationWorkerUnavailable;
  }

  if (draft.prompt.trim().length === 0) {
    validationErrors.prompt = messages.editableConfigurationPromptRequired;
  } else if (promptValidationState.status === "loading") {
    validationErrors.prompt =
      messages.editableConfigurationPromptValidationLoading;
  } else if (promptValidationState.status === "error") {
    validationErrors.prompt = `${messages.editableConfigurationPromptValidationErrorPrefix} ${promptValidationState.errorMessage}`;
  } else if (
    promptValidationState.status === "ready" &&
    promptValidationState.diagnostics.length > 0
  ) {
    validationErrors.prompt =
      messages.editableConfigurationPromptDiagnosticsSummary;
  }

  return validationErrors;
}

export function hasEditableWorkstationValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
): boolean {
  return Boolean(validationErrors.prompt || validationErrors.workerName);
}

function areEditableDraftsEqual(
  left: EditableWorkstationDraft,
  right: EditableWorkstationDraft,
): boolean {
  return (
    left.prompt === right.prompt &&
    left.runnerName === right.runnerName &&
    left.workerName === right.workerName
  );
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
    ReturnType<typeof useCurrentEditableFactoryDefinition>["data"]
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
