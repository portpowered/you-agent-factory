import { useEffect, useMemo, useState } from "react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import {
  applyEditableWorkerDraft,
  areEditableWorkerDraftsEqual,
  type EditableWorkerDraft,
  editableWorkerDraftFromValues,
  resolveEditableWorkerValues,
} from "../../../current-factory-definition/lib/worker-editable-values";
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
  editableDefinition?: CurrentFactoryDocument | null,
): EditableWorkerConfigurationState | undefined {
  const isWorkerSelection = selection?.kind === "worker" && workerName != null;
  const messages = getWorkerDetailMessages(locale);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableWorkerSession(editableDefinition, workerName, isWorkerSelection);

  if (!isWorkerSelection || !workerName) {
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

  return buildReadyEditableWorkerConfigurationState({
    editableDefinition,
    sessionState,
    setSessionState,
    selectedEditableValues,
    workerName,
  });
}

function useEditableWorkerSession(
  editableDefinition: CurrentFactoryDocument | null | undefined,
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
  editableDefinition: NonNullable<CurrentFactoryDocument>;
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
  const workerIndex =
    editableDefinition.workers?.findIndex(
      (worker) => worker.name === workerName,
    ) ?? -1;
  const pendingFactoryDefinition = applyEditableWorkerDraft(
    editableDefinition,
    workerName,
    sessionState.draft,
  );
  const messages = getWorkerDetailMessages();
  const validationErrors = mergeEditableWorkerContractValidationErrors(
    validateEditableWorkerDraft(sessionState.draft, messages, {
      originalWorkerName: workerName,
      workerNames: (editableDefinition.workers ?? []).map(
        (worker) => worker.name,
      ),
    }),
    pendingFactoryDefinition,
    workerName,
    messages,
    workerIndex >= 0 ? workerIndex : undefined,
  );
  const hasValidationErrors =
    hasEditableWorkerValidationErrors(validationErrors);
  const isDirty = !areEditableWorkerDraftsEqual(
    sessionState.draft,
    sessionState.sessionStartDraft,
  );
  const draftHandlers = createEditableWorkerDraftHandlers(setSessionState);

  return {
    baseVersion: editableDefinition.version,
    canSave:
      isDirty && !hasValidationErrors && pendingFactoryDefinition != null,
    draft: sessionState.draft,
    hasValidationErrors,
    initialValues: selectedEditableValues,
    isDirty,
    ...draftHandlers,
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
    overwriteFieldNames: resolveEditableWorkerOverwriteFields(
      sessionState.sessionStartDraft,
      sessionState.draft,
      sessionState.latestDefinitionDraft,
    ),
    pendingFactoryDefinition,
    savedFactoryDefinition: editableDefinition,
    status: "ready",
    validationErrors,
  };
}

function createEditableWorkerDraftHandlers(
  setSessionState: (
    updater: (
      currentState: EditableWorkerSessionState | null,
    ) => EditableWorkerSessionState | null,
  ) => void,
) {
  return {
    onArgsTextChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, argsText: value }));
    },
    onAuthSecretRefChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        authSecretRef: value,
      }));
    },
    onBodyChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, body: value }));
    },
    onCommandChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, command: value }));
    },
    onExecutorProviderChange: (
      value: EditableWorkerDraft["executorProvider"],
    ) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        executorProvider: value,
      }));
    },
    onLinearClaimAssigneeFieldChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        linearClaimAssigneeField: value,
      }));
    },
    onLinearMappingStateChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        linearMappingState: value,
      }));
    },
    onLinearMappingWorkTypeChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        linearMappingWorkType: value,
      }));
    },
    onLinearPollIntervalChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        linearPollInterval: value,
      }));
    },
    onLinearStateIdsTextChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        linearStateIdsText: value,
      }));
    },
    onLinearTeamIdsTextChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        linearTeamIdsText: value,
      }));
    },
    onModelChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, model: value }));
    },
    onModelLocalityChange: (value: EditableWorkerDraft["modelLocality"]) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        modelLocality: value,
      }));
    },
    onModelProviderChange: (value: EditableWorkerDraft["modelProvider"]) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        modelProvider: value,
      }));
    },
    onNameChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, name: value }));
    },
    onProviderChange: (value: EditableWorkerDraft["provider"]) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, provider: value }));
    },
    onSkipPermissionsChange: (value: boolean) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        skipPermissions: value,
      }));
    },
    onStopTokenChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, stopToken: value }));
    },
    onTimeoutAmountChange: (value: string) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        timeoutAmount: value,
      }));
    },
    onTimeoutUnitChange: (value: EditableWorkerDraft["timeoutUnit"]) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        timeoutUnit: value,
      }));
    },
    onTypeChange: (value: EditableWorkerDraft["type"]) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, type: value }));
    },
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
