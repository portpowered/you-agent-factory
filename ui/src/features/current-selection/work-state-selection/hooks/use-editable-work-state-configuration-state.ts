import { useEffect, useMemo, useState } from "react";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
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
  editableDefinition?: CurrentFactoryDocument | null,
): EditableWorkStateConfigurationState | undefined {
  const isStateNodeSelection =
    selection?.kind === "state-node" && placeId != null;
  const messages = getWorkStateDetailMessages(locale);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkStateSession(
      editableDefinition,
      placeId,
      isStateNodeSelection,
    );

  if (!isStateNodeSelection || !placeId) {
    return undefined;
  }

  if (!editableDefinition) {
    return { status: "loading" };
  }

  if (!selectedEditableValues || !sessionState) {
    return {
      message: messages.configurationEmpty,
      status: "empty",
    };
  }

  return buildReadyEditableWorkStateConfigurationState({
    editableDefinition,
    placeId,
    selectedEditableValues,
    sessionState,
    setSessionState,
  });
}

function useEditableWorkStateSession(
  editableDefinition: CurrentFactoryDocument | null | undefined,
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
  editableDefinition: NonNullable<CurrentFactoryDocument>;
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
    savedFactoryDefinition: editableDefinition,
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
