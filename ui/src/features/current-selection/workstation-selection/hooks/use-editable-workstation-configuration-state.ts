import { useEffect, useMemo, useState } from "react";

import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { workstationBehaviorRequiresPrompt } from "../../../current-factory-definition/lib/workstation-behavior";
import {
  applyEditableWorkstationDraft,
  areEditableWorkstationDraftsEqual,
  type EditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import type { DashboardSelection } from "../../base/state/selection-types";
import {
  buildEditableWorkstationConfigurationMutators,
  type EditableWorkstationSessionDraftState,
} from "../editing/editable-workstation-configuration-mutators";
import {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "../editing/editable-workstation-draft-validation";
import { resolveEditableWorkstationOverwriteFields } from "../editing/editable-workstation-overwrite-fields";
import {
  resolvePromptHelpState,
  resolvePromptValidationState,
} from "../editing/editable-workstation-prompt-state";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationPromptHelpState,
  EditableWorkstationPromptValidationState,
  EditableWorkstationValidationErrors,
  EditableWorkstationWorkerOptionsState,
} from "../lib/detail-card-types";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../messages/workstation-detail";
import { useCurrentWorkstationPromptTemplateContract } from "./useCurrentWorkstationPromptTemplateContract";
import { useCurrentWorkstationPromptTemplateValidation } from "./useCurrentWorkstationPromptTemplateValidation";

type EditableWorkstationSessionState = EditableWorkstationSessionDraftState & {
  selectionKey: string;
};

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

export {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "../editing/editable-workstation-draft-validation";

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
  const mutators = buildEditableWorkstationConfigurationMutators({
    selectedEditableValues,
    setSessionState,
  });

  return {
    baseVersion: editableDefinition.version,
    draft: sessionState.draft,
    hasValidationErrors: hasEditableWorkstationValidationErrors(
      resolvedValidationErrors,
    ),
    initialValues: selectedEditableValues,
    isDirty: !areEditableWorkstationDraftsEqual(
      sessionState.draft,
      sessionState.sessionStartDraft,
    ),
    ...mutators,
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
    areEditableWorkstationDraftsEqual(
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

  return areEditableWorkstationDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
