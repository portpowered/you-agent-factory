import { useEffect, useMemo, useRef, useState } from "react";

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
} from "./detail-card-types";
import type { DashboardSelection } from "./types";

const EMPTY_EDITABLE_CONFIGURATION_MESSAGE =
  "This running factory definition does not expose editable prompt, model, and template values for the selected workstation.";

interface EditableWorkstationSessionState {
  draft: EditableWorkstationDraft;
  selectionKey: string;
  sessionStartDraft: EditableWorkstationDraft;
}

export function useEditableWorkstationConfigurationState(
  selection: DashboardSelection | null,
  selectedNode: DashboardWorkstationNode | null,
): EditableWorkstationConfigurationState | undefined {
  const editableDefinitionEnabled = useEditableDefinitionGate(
    selection,
    selectedNode,
  );
  const editableDefinition = useCurrentEditableFactoryDefinition(
    editableDefinitionEnabled &&
      selection?.kind === "node" &&
      selectedNode != null,
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
      message: EMPTY_EDITABLE_CONFIGURATION_MESSAGE,
      status: "empty",
    };
  }

  const validationErrors = validateEditableWorkstationDraft(sessionState.draft);
  const pendingFactoryDefinition = hasEditableWorkstationValidationErrors(
    validationErrors,
  )
    ? null
    : applyEditableWorkstationDraft(
        editableDefinition.data,
        selectedNode,
        sessionState.draft,
      );

  return {
    draft: sessionState.draft,
    hasValidationErrors:
      hasEditableWorkstationValidationErrors(validationErrors),
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
    onModelChange: (value) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                model: value,
              },
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
    onPromptFileChange: (value) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                promptFile: value,
              },
            }
          : currentState,
      );
    },
    pendingFactoryDefinition,
    status: "ready",
    validationErrors,
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
): EditableWorkstationValidationErrors {
  const validationErrors: EditableWorkstationValidationErrors = {};

  if (draft.model.trim().length === 0) {
    validationErrors.model = "Enter a model before saving this workstation.";
  }

  if (draft.prompt.trim().length === 0) {
    validationErrors.prompt = "Enter a prompt before saving this workstation.";
  }

  if (draft.promptFile.length > 0 && draft.promptFile.trim().length === 0) {
    validationErrors.promptFile =
      "Template paths cannot be only whitespace. Clear the field to remove the template.";
  }

  return validationErrors;
}

export function hasEditableWorkstationValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
): boolean {
  return Boolean(
    validationErrors.model ||
      validationErrors.prompt ||
      validationErrors.promptFile,
  );
}

function areEditableDraftsEqual(
  left: EditableWorkstationDraft,
  right: EditableWorkstationDraft,
): boolean {
  return (
    left.model === right.model &&
    left.prompt === right.prompt &&
    left.promptFile === right.promptFile
  );
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
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  return areEditableDraftsEqual(
    currentState.draft,
    currentState.sessionStartDraft,
  )
    ? {
        draft: initialDraft,
        selectionKey,
        sessionStartDraft: initialDraft,
      }
    : currentState;
}

function useEditableDefinitionGate(
  selection: DashboardSelection | null,
  selectedNode: DashboardWorkstationNode | null,
) {
  const [editableDefinitionEnabled, setEditableDefinitionEnabled] =
    useState(false);
  const previousSelectedNodeID = useRef<string | null>(null);
  const hasMounted = useRef(false);

  useEffect(() => {
    const selectedNodeID =
      selection?.kind === "node" && selectedNode ? selectedNode.node_id : null;

    if (!hasMounted.current) {
      hasMounted.current = true;
      previousSelectedNodeID.current = selectedNodeID;
      return;
    }

    if (selectedNodeID && selectedNodeID !== previousSelectedNodeID.current) {
      setEditableDefinitionEnabled(true);
    }

    if (!selectedNodeID) {
      setEditableDefinitionEnabled(false);
    }

    previousSelectedNodeID.current = selectedNodeID;
  }, [selectedNode, selection]);

  return editableDefinitionEnabled;
}
