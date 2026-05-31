import { useEffect, useMemo, useState } from "react";
import {
  applyEditableWorkerDraft,
  type EditableWorkerDraft,
  editableWorkerDraftFromValues,
  resolveEditableWorkerValues,
} from "../../../current-factory-definition/lib/worker-editable-values";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { resolveEditableWorkerOverwriteFields } from "../editing/editable-worker-overwrite-fields";
import type { EditableWorkerConfigurationState } from "../lib/detail-card-types";
import {
  hasEditableWorkerValidationErrors,
  mergeEditableWorkerContractValidationErrors,
  validateEditableWorkerDraft,
} from "../lib/worker-editable-validation";
import { getWorkerDetailMessages } from "../messages/worker-detail";

interface EditableWorkerSessionState {
  draft: EditableWorkerDraft;
  latestDefinitionDraft: EditableWorkerDraft;
  selectionKey: string;
  sessionStartDraft: EditableWorkerDraft;
}

export function useEditableWorkerConfigurationState(
  selection: DashboardSelection | null,
  workerName: string | null,
  locale?: string | null,
): EditableWorkerConfigurationState | undefined {
  const isWorkerSelection = selection?.kind === "worker" && workerName != null;
  const messages = getWorkerDetailMessages(locale);
  const editableDefinition = useCurrentFactoryDocument(isWorkerSelection);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkerSession(
      editableDefinition.data,
      workerName,
      isWorkerSelection,
    );

  if (!isWorkerSelection || !workerName) {
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

  return buildReadyEditableWorkerConfigurationState({
    editableDefinition: editableDefinition.data,
    sessionState,
    setSessionState,
    selectedEditableValues,
    workerName,
  });
}

function useEditableWorkerSession(
  editableDefinition: ReturnType<typeof useCurrentFactoryDocument>["data"],
  workerName: string | null,
  isWorkerSelection: boolean,
) {
  const selectedEditableValues = useMemo(() => {
    if (!isWorkerSelection || !workerName || !editableDefinition) {
      return null;
    }

    return resolveEditableWorkerValues(editableDefinition, workerName);
  }, [editableDefinition, isWorkerSelection, workerName]);
  const selectionKey = isWorkerSelection && workerName ? workerName : null;
  const [sessionState, setSessionState] =
    useState<EditableWorkerSessionState | null>(null);

  useEffect(() => {
    setSessionState((currentState) =>
      syncEditableWorkerSession(
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

function buildReadyEditableWorkerConfigurationState({
  editableDefinition,
  sessionState,
  setSessionState,
  selectedEditableValues,
  workerName,
}: {
  editableDefinition: NonNullable<
    ReturnType<typeof useCurrentFactoryDocument>["data"]
  >;
  sessionState: EditableWorkerSessionState;
  setSessionState: (
    updater: (
      currentState: EditableWorkerSessionState | null,
    ) => EditableWorkerSessionState | null,
  ) => void;
  selectedEditableValues: NonNullable<
    ReturnType<typeof resolveEditableWorkerValues>
  >;
  workerName: string;
}): Extract<EditableWorkerConfigurationState, { status: "ready" }> {
  const pendingFactoryDefinition = applyEditableWorkerDraft(
    editableDefinition,
    workerName,
    sessionState.draft,
  );
  const messages = getWorkerDetailMessages();
  const validationErrors = mergeEditableWorkerContractValidationErrors(
    validateEditableWorkerDraft(sessionState.draft, messages),
    pendingFactoryDefinition,
    workerName,
    messages,
  );
  const hasValidationErrors =
    hasEditableWorkerValidationErrors(validationErrors);
  const isDirty = !areEditableWorkerDraftsEqual(
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
    onArgsTextChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, argsText: value }));
    },
    onBodyChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, body: value }));
    },
    onCommandChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, command: value }));
    },
    onExecutorProviderChange: (value) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        executorProvider: value,
      }));
    },
    onModelChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, model: value }));
    },
    onModelLocalityChange: (value) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        modelLocality: value,
      }));
    },
    onModelProviderChange: (value) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        modelProvider: value,
      }));
    },
    onProviderChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, provider: value }));
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
    onTypeChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, type: value }));
    },
    overwriteFieldNames: resolveEditableWorkerOverwriteFields(
      sessionState.sessionStartDraft,
      sessionState.draft,
      sessionState.latestDefinitionDraft,
    ),
    pendingFactoryDefinition,
    status: "ready",
    validationErrors,
  };
}

function updateDraft(
  setSessionState: (
    updater: (
      currentState: EditableWorkerSessionState | null,
    ) => EditableWorkerSessionState | null,
  ) => void,
  updater: (draft: EditableWorkerDraft) => EditableWorkerDraft,
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

function areEditableWorkerDraftsEqual(
  left: EditableWorkerDraft,
  right: EditableWorkerDraft,
): boolean {
  return (
    left.argsText === right.argsText &&
    left.body === right.body &&
    left.command === right.command &&
    left.executorProvider === right.executorProvider &&
    left.model === right.model &&
    left.modelLocality === right.modelLocality &&
    left.modelProvider === right.modelProvider &&
    left.provider === right.provider &&
    left.type === right.type
  );
}

function syncEditableWorkerSession(
  currentState: EditableWorkerSessionState | null,
  selectedEditableValues: ReturnType<typeof resolveEditableWorkerValues>,
  selectionKey: string | null,
): EditableWorkerSessionState | null {
  if (!selectionKey || !selectedEditableValues) {
    return null;
  }

  const initialDraft = editableWorkerDraftFromValues(selectedEditableValues);
  if (!currentState || currentState.selectionKey !== selectionKey) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  if (
    areEditableWorkerDraftsEqual(
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

  return areEditableWorkerDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
