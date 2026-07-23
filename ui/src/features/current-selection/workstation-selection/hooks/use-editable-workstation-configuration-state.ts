import { useEffect, useMemo, useState } from "react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import {
  isModelInvokeWorkstationType,
  workstationUsesPromptOrientedEditing,
} from "../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import { workstationBehaviorRequiresPrompt } from "../../../current-factory-definition/lib/workstation-behavior";
import {
  type EditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { editableWorkstationDraftsEqual } from "../../../current-factory-definition/lib/workstation-guards";
import { workstationRequiresWorkerAssignment } from "../../../current-factory-definition/lib/workstation-worker-assignment";
import type { DashboardSelection } from "../../base/state/selection-types";
import {
  resolvePromptHelpState,
  resolvePromptValidationState,
} from "../editing/editable-workstation-prompt-state";
import type { EditableWorkstationConfigurationState } from "../lib/keys/detail-card-types";
import { validateEditableWorkstationDraft } from "../lib/validation/editable-workstation-configuration-validation";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { buildReadyEditableWorkstationConfigurationState } from "./editable-workstation-ready-configuration-state";
import { useCurrentWorkstationPromptTemplateContract } from "./useCurrentWorkstationPromptTemplateContract";
import { useCurrentWorkstationPromptTemplateValidation } from "./useCurrentWorkstationPromptTemplateValidation";

export {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "../lib/validation/editable-workstation-configuration-validation";

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
  editableDefinition?: CurrentFactoryDocument | null,
): EditableWorkstationConfigurationState | undefined {
  const isNodeSelection = selection?.kind === "node" && selectedNode != null;
  const messages = getWorkstationDetailMessages(locale);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkstationSession(editableDefinition, selectedNode, selection);
  const draftWorkstationType =
    sessionState?.draft.workstationType ??
    selectedEditableValues?.workstationType;
  const usesPromptOrientedEditing =
    draftWorkstationType != null &&
    workstationUsesPromptOrientedEditing(draftWorkstationType);
  const promptTemplateContract = useCurrentWorkstationPromptTemplateContract(
    selectedEditableValues?.workstationName,
    isNodeSelection &&
      selectedEditableValues != null &&
      usesPromptOrientedEditing,
  );
  const shouldValidatePrompt =
    isNodeSelection &&
    sessionState != null &&
    selectedEditableValues != null &&
    draftWorkstationType != null &&
    !isModelInvokeWorkstationType(draftWorkstationType) &&
    workstationRequiresWorkerAssignment({
      type: draftWorkstationType,
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

  if (!editableDefinition) {
    return { status: "loading" };
  }

  if (!selectedEditableValues || !sessionState) {
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
    {
      originalWorkstationName: selectedEditableValues.workstationName,
      workstationNames: (editableDefinition.workstations ?? []).map(
        (workstation) => workstation.name,
      ),
    },
  );

  return buildReadyEditableWorkstationConfigurationState({
    editableDefinition,
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
  editableDefinition: CurrentFactoryDocument | null | undefined,
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
