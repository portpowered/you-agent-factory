import { useEffect, useMemo, useState } from "react";

import type { DashboardWorkstationNode } from "../../api/dashboard/types";
import { useCurrentEditableFactoryDefinition } from "../current-factory-definition";
import {
  applyEditableWorkstationDraft,
  type EditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../current-factory-definition/workstation-editable-values";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationValidationErrors,
  EditableWorkstationWorkerOptionsState,
} from "./detail-card-types";
import { resolveEditableWorkstationOverwriteFields } from "./editable-workstation-overwrite-fields";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "./messages";
import type { DashboardSelection } from "./types";

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
  const messages = getWorkstationDetailMessages(locale);
  const editableDefinition = useCurrentEditableFactoryDefinition(
    selection?.kind === "node" && selectedNode != null,
  );
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkstationSession(
      editableDefinition.data,
      selectedNode,
      selection,
    );

  if (selection?.kind !== "node" || !selectedNode) {
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

  const resolvedValidationErrors = validateEditableWorkstationDraft(
    sessionState.draft,
    selectedEditableValues,
    messages,
  );
  const pendingFactoryDefinition = hasEditableWorkstationValidationErrors(
    resolvedValidationErrors,
  )
    ? null
    : applyEditableWorkstationDraft(
        editableDefinition.data,
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
    onPromptChange: (value) => {
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
    onWorkerChange: (value) => {
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
    status: "ready",
    validationErrors: resolvedValidationErrors,
    workerOptionsState: resolveWorkerOptionsState(
      sessionState.draft,
      selectedEditableValues,
      messages,
    ),
  };
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
  messages: Pick<
    WorkstationDetailMessages,
    | "editableConfigurationPromptRequired"
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
  return left.prompt === right.prompt && left.workerName === right.workerName;
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
