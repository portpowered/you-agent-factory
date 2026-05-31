import { useEffect, useMemo, useState } from "react";

import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import {
  applyEditableWorkStateDraft,
  type EditableWorkStateDraft,
  editableWorkStateDraftFromValues,
  resolveEditableWorkStateValues,
} from "../../../current-factory-definition/lib/work-state-editable-values";
import type { DashboardSelection } from "../../base/state/selection-types";
import type { EditableWorkStateConfigurationState } from "../lib/detail-card-types";
import {
  hasEditableWorkStateValidationErrors,
  mergeEditableWorkStateContractValidationErrors,
  validateEditableWorkStateDraft,
} from "../lib/work-state-editable-validation";
import { getWorkStateDetailMessages } from "../messages/work-state-detail";

interface EditableWorkStateSessionState {
  draft: EditableWorkStateDraft;
  latestDefinitionDraft: EditableWorkStateDraft;
  selectionKey: string;
  sessionStartDraft: EditableWorkStateDraft;
}

export function useEditableWorkStateConfigurationState(
  selection: DashboardSelection | null,
  placeId: string | null,
  locale?: string | null,
): EditableWorkStateConfigurationState | undefined {
  const isStateNodeSelection =
    selection?.kind === "state-node" && placeId != null;
  const messages = getWorkStateDetailMessages(locale);
  const editableDefinition = useCurrentFactoryDocument(isStateNodeSelection);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkStateSession(
      editableDefinition.data,
      placeId,
      isStateNodeSelection,
    );

  if (!isStateNodeSelection || !placeId) {
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
      message: messages.configurationEmpty,
      status: "empty",
    };
  }

  return buildReadyEditableWorkStateConfigurationState({
    editableDefinition: editableDefinition.data,
    placeId,
    selectedEditableValues,
    sessionState,
    setSessionState,
  });
}

function useEditableWorkStateSession(
  editableDefinition: ReturnType<typeof useCurrentFactoryDocument>["data"],
  placeId: string | null,
  isStateNodeSelection: boolean,
) {
  const selectedEditableValues = useMemo(() => {
    if (!isStateNodeSelection || !placeId || !editableDefinition) {
      return null;
    }

    return resolveEditableWorkStateValues(editableDefinition, placeId);
  }, [editableDefinition, isStateNodeSelection, placeId]);
  const selectionKey = isStateNodeSelection && placeId ? placeId : null;
  const [sessionState, setSessionState] =
    useState<EditableWorkStateSessionState | null>(null);

  useEffect(() => {
    setSessionState((currentState) =>
      syncEditableWorkStateSession(
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

function buildReadyEditableWorkStateConfigurationState({
  editableDefinition,
  placeId,
  selectedEditableValues,
  sessionState,
  setSessionState,
}: {
  editableDefinition: NonNullable<
    ReturnType<typeof useCurrentFactoryDocument>["data"]
  >;
  placeId: string;
  selectedEditableValues: NonNullable<
    ReturnType<typeof resolveEditableWorkStateValues>
  >;
  sessionState: EditableWorkStateSessionState;
  setSessionState: (
    updater: (
      currentState: EditableWorkStateSessionState | null,
    ) => EditableWorkStateSessionState | null,
  ) => void;
}): Extract<EditableWorkStateConfigurationState, { status: "ready" }> {
  const messages = getWorkStateDetailMessages();
  const pendingFactoryDefinition = applyEditableWorkStateDraft(
    editableDefinition,
    placeId,
    sessionState.draft,
  );
  const validationErrors = mergeEditableWorkStateContractValidationErrors(
    validateEditableWorkStateDraft(sessionState.draft, messages, {
      originalStateName: selectedEditableValues.stateName,
      stateNamesInWorkType: selectedEditableValues.stateNamesInWorkType,
    }),
    pendingFactoryDefinition,
    messages,
  );
  const hasValidationErrors =
    hasEditableWorkStateValidationErrors(validationErrors);
  const isDirty = !areEditableWorkStateDraftsEqual(
    sessionState.draft,
    sessionState.sessionStartDraft,
  );

  return {
    baseVersion: editableDefinition.version,
    canSave:
      isDirty && !hasValidationErrors && pendingFactoryDefinition != null,
    draft: sessionState.draft,
    hasValidationErrors,
    initialValues: selectedEditableValues,
    isDirty,
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
    onNameChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, name: value }));
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
    originalStateName: selectedEditableValues.stateName,
    pendingFactoryDefinition,
    status: "ready",
    validationErrors,
    workTypeName: selectedEditableValues.workTypeName,
  };
}

function updateDraft(
  setSessionState: (
    updater: (
      currentState: EditableWorkStateSessionState | null,
    ) => EditableWorkStateSessionState | null,
  ) => void,
  updater: (draft: EditableWorkStateDraft) => EditableWorkStateDraft,
) {
  setSessionState((currentState) =>
    currentState
      ? {
          ...currentState,
          draft: updater(currentState.draft),
        }
      : currentState,
  );
}

function areEditableWorkStateDraftsEqual(
  left: EditableWorkStateDraft,
  right: EditableWorkStateDraft,
): boolean {
  return left.name === right.name && left.type === right.type;
}

function syncEditableWorkStateSession(
  currentState: EditableWorkStateSessionState | null,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkStateValues>,
  selectionKey: string | null,
): EditableWorkStateSessionState | null {
  if (!selectionKey || !selectedEditableValues) {
    return null;
  }

  const initialDraft = editableWorkStateDraftFromValues(selectedEditableValues);
  if (!currentState || currentState.selectionKey !== selectionKey) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  if (
    areEditableWorkStateDraftsEqual(
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

  return areEditableWorkStateDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
