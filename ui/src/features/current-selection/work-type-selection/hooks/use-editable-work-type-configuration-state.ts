import { useEffect, useMemo, useState } from "react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import {
  hasEditableWorkTypeValidationErrors,
  mergeEditableWorkTypeContractValidationErrors,
  validateEditableWorkTypeDraft,
} from "../../../current-factory-definition/lib/work-type-editable-validation";
import {
  applyEditableWorkTypeDraft,
  type EditableWorkTypeDraft,
  editableWorkTypeDraftFromValues,
  resolveEditableWorkTypeValues,
} from "../../../current-factory-definition/lib/work-type-editable-values";
import type { DashboardSelection } from "../../base/state/selection-types";
import type { EditableWorkTypeConfigurationState } from "../lib/detail-card-types";
import { getWorkTypeDetailMessages } from "../messages/work-type-detail";

interface EditableWorkTypeSessionState {
  draft: EditableWorkTypeDraft;
  latestDefinitionDraft: EditableWorkTypeDraft;
  selectionKey: string;
  sessionStartDraft: EditableWorkTypeDraft;
}

export function useEditableWorkTypeConfigurationState(
  selection: DashboardSelection | null,
  workTypeName: string | null,
  locale?: string | null,
  editableDefinition?: CurrentFactoryDocument | null,
): EditableWorkTypeConfigurationState | undefined {
  const isWorkTypeSelection =
    selection?.kind === "work-type" && workTypeName != null;
  const messages = getWorkTypeDetailMessages(locale);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkTypeSession(
      editableDefinition,
      workTypeName,
      isWorkTypeSelection,
    );

  if (!isWorkTypeSelection || !workTypeName) {
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

  return buildReadyEditableWorkTypeConfigurationState({
    editableDefinition,
    messages,
    sessionState,
    setSessionState,
    selectedEditableValues,
    workTypeName,
  });
}

function useEditableWorkTypeSession(
  editableDefinition: CurrentFactoryDocument | null | undefined,
  workTypeName: string | null,
  isWorkTypeSelection: boolean,
) {
  const selectedEditableValues = useMemo(() => {
    if (!isWorkTypeSelection || !workTypeName || !editableDefinition) {
      return null;
    }

    return resolveEditableWorkTypeValues(editableDefinition, workTypeName);
  }, [editableDefinition, isWorkTypeSelection, workTypeName]);
  const selectionKey =
    isWorkTypeSelection && workTypeName ? workTypeName : null;
  const [sessionState, setSessionState] =
    useState<EditableWorkTypeSessionState | null>(null);

  useEffect(() => {
    setSessionState((currentState) =>
      syncEditableWorkTypeSession(
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

function buildReadyEditableWorkTypeConfigurationState({
  editableDefinition,
  messages,
  sessionState,
  setSessionState,
  selectedEditableValues,
  workTypeName,
}: {
  editableDefinition: NonNullable<CurrentFactoryDocument>;
  messages: ReturnType<typeof getWorkTypeDetailMessages>;
  sessionState: EditableWorkTypeSessionState;
  setSessionState: (
    updater: (
      currentState: EditableWorkTypeSessionState | null,
    ) => EditableWorkTypeSessionState | null,
  ) => void;
  selectedEditableValues: NonNullable<
    ReturnType<typeof resolveEditableWorkTypeValues>
  >;
  workTypeName: string;
}): Extract<EditableWorkTypeConfigurationState, { status: "ready" }> {
  const pendingFactoryDefinition = applyEditableWorkTypeDraft(
    editableDefinition,
    workTypeName,
    sessionState.draft,
  );
  const validationErrors = mergeEditableWorkTypeContractValidationErrors(
    validateEditableWorkTypeDraft(sessionState.draft, messages, {
      originalWorkTypeName: workTypeName,
      workTypeNames: (editableDefinition.workTypes ?? []).map(
        (workType) => workType.name,
      ),
    }),
    pendingFactoryDefinition,
    messages,
  );
  const hasValidationErrors =
    hasEditableWorkTypeValidationErrors(validationErrors);
  const isDirty = !areEditableWorkTypeDraftsEqual(
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
    onHandlingBehaviorChange: (value) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        handlingBehavior: value,
      }));
    },
    onNameChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, name: value }));
    },
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
    pendingFactoryDefinition,
    savedFactoryDefinition: editableDefinition,
    status: "ready",
    validationErrors,
  };
}

function updateDraft(
  setSessionState: (
    updater: (
      currentState: EditableWorkTypeSessionState | null,
    ) => EditableWorkTypeSessionState | null,
  ) => void,
  updater: (draft: EditableWorkTypeDraft) => EditableWorkTypeDraft,
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

function areEditableWorkTypeDraftsEqual(
  left: EditableWorkTypeDraft,
  right: EditableWorkTypeDraft,
): boolean {
  return (
    left.name === right.name &&
    areHandlingBehaviorsEqual(left.handlingBehavior, right.handlingBehavior)
  );
}

function areHandlingBehaviorsEqual(
  left: EditableWorkTypeDraft["handlingBehavior"],
  right: EditableWorkTypeDraft["handlingBehavior"],
): boolean {
  if (left === right) {
    return true;
  }
  if (left == null || right == null) {
    return left === right;
  }
  if (left.length !== right.length) {
    return false;
  }
  return left.every((value, index) => value === right[index]);
}

function syncEditableWorkTypeSession(
  currentState: EditableWorkTypeSessionState | null,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkTypeValues>,
  selectionKey: string | null,
): EditableWorkTypeSessionState | null {
  if (!selectionKey || !selectedEditableValues) {
    return null;
  }

  const initialDraft = editableWorkTypeDraftFromValues(selectedEditableValues);
  if (!currentState || currentState.selectionKey !== selectionKey) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  if (
    areEditableWorkTypeDraftsEqual(
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

  return areEditableWorkTypeDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
