import { useEffect, useMemo, useState } from "react";

import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { workstationBehaviorRequiresPrompt } from "../../../current-factory-definition/lib/workstation-behavior";
import {
  type EditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { editableWorkstationDraftsEqual } from "../../../current-factory-definition/lib/workstation-guards";
import type { DashboardSelection } from "../../base/state/selection-types";
import {
  resolvePromptHelpState,
  resolvePromptValidationState,
} from "../editing/editable-workstation-prompt-state";
import type { EditableWorkstationConfigurationState } from "../lib/detail-card-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { useCurrentWorkstationPromptTemplateContract } from "./useCurrentWorkstationPromptTemplateContract";
import { useCurrentWorkstationPromptTemplateValidation } from "./useCurrentWorkstationPromptTemplateValidation";
import { validateEditableWorkstationDraft } from "./editable-workstation-draft-validation";
import { buildReadyEditableWorkstationConfigurationState } from "./editable-workstation-ready-configuration-state";

export {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "./editable-workstation-draft-validation";

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
    editableWorkstationDraftsEqual(
      currentState.draft,
      currentState.sessionStartDraft,
    )
  ) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  return editableWorkstationDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
