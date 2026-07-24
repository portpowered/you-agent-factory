import { useEffect, useMemo, useState } from "react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import {
  applyEditableResourceDraft,
  type EditableResourceDraft,
  editableResourceDraftFromValues,
  resolveEditableResourceValues,
} from "../../../current-factory-definition/lib/resource-editable-values";
import type { DashboardSelection } from "../../base/state/selection-types";
import { resolveEditableResourceOverwriteFields } from "../editing/editable-resource-overwrite-fields";
import type { EditableResourceConfigurationState } from "../lib/detail-card-types";
import {
  hasEditableResourceValidationErrors,
  mergeEditableResourceContractValidationErrors,
  validateEditableResourceDraft,
} from "../lib/resource-editable-validation";
import { getResourceDetailMessages } from "../messages/resource-detail";

interface EditableResourceSessionState {
  draft: EditableResourceDraft;
  latestDefinitionDraft: EditableResourceDraft;
  selectionKey: string;
  sessionStartDraft: EditableResourceDraft;
}

export function useEditableResourceConfigurationState(
  selection: DashboardSelection | null,
  resourceName: string | null,
  locale?: string | null,
  editableDefinition?: CurrentFactoryDocument | null,
): EditableResourceConfigurationState | undefined {
  const isResourceSelection =
    selection?.kind === "resource" && resourceName != null;
  const messages = getResourceDetailMessages(locale);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableResourceSession(
      editableDefinition,
      resourceName,
      isResourceSelection,
    );

  if (!isResourceSelection || !resourceName) {
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

  return buildReadyEditableResourceConfigurationState({
    editableDefinition,
    resourceName,
    sessionState,
    setSessionState,
    selectedEditableValues,
  });
}

function useEditableResourceSession(
  editableDefinition: CurrentFactoryDocument | null | undefined,
  resourceName: string | null,
  isResourceSelection: boolean,
) {
  const selectedEditableValues = useMemo(() => {
    if (!isResourceSelection || !resourceName || !editableDefinition) {
      return null;
    }

    return resolveEditableResourceValues(editableDefinition, resourceName);
  }, [editableDefinition, isResourceSelection, resourceName]);
  const selectionKey =
    isResourceSelection && resourceName ? resourceName : null;
  const [sessionState, setSessionState] =
    useState<EditableResourceSessionState | null>(null);

  useEffect(() => {
    setSessionState((currentState) =>
      syncEditableResourceSession(
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

function buildReadyEditableResourceConfigurationState({
  editableDefinition,
  resourceName,
  sessionState,
  setSessionState,
  selectedEditableValues,
}: {
  editableDefinition: NonNullable<CurrentFactoryDocument>;
  resourceName: string;
  sessionState: EditableResourceSessionState;
  setSessionState: (
    updater: (
      currentState: EditableResourceSessionState | null,
    ) => EditableResourceSessionState | null,
  ) => void;
  selectedEditableValues: NonNullable<
    ReturnType<typeof resolveEditableResourceValues>
  >;
}): Extract<EditableResourceConfigurationState, { status: "ready" }> {
  const resourceIndex =
    editableDefinition.resources?.findIndex(
      (resource) => resource.name === resourceName,
    ) ?? -1;
  const pendingFactoryDefinition = applyEditableResourceDraft(
    editableDefinition,
    resourceName,
    sessionState.draft,
  );
  const messages = getResourceDetailMessages();
  const validationErrors = mergeEditableResourceContractValidationErrors(
    validateEditableResourceDraft(sessionState.draft, messages, {
      originalResourceName: resourceName,
      resourceNames: (editableDefinition.resources ?? []).map(
        (resource) => resource.name,
      ),
    }),
    pendingFactoryDefinition,
    resourceName,
    messages,
    resourceIndex >= 0 ? resourceIndex : undefined,
  );
  const hasValidationErrors =
    hasEditableResourceValidationErrors(validationErrors);
  const isDirty = !areEditableResourceDraftsEqual(
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
    onBackendChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, backend: value }));
    },
    onCapacityChange: (value) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        capacityText: value,
      }));
    },
    onLoadPolicyChange: (value) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        loadPolicy: value,
      }));
    },
    onModelChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, model: value }));
    },
    onNameChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, name: value }));
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
    overwriteFieldNames: resolveEditableResourceOverwriteFields(
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

function updateDraft(
  setSessionState: (
    updater: (
      currentState: EditableResourceSessionState | null,
    ) => EditableResourceSessionState | null,
  ) => void,
  updater: (draft: EditableResourceDraft) => EditableResourceDraft,
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

function areEditableResourceDraftsEqual(
  left: EditableResourceDraft,
  right: EditableResourceDraft,
): boolean {
  return (
    left.backend === right.backend &&
    left.capacityText === right.capacityText &&
    left.loadPolicy === right.loadPolicy &&
    left.model === right.model &&
    left.name === right.name &&
    left.provider === right.provider &&
    left.type === right.type
  );
}

function syncEditableResourceSession(
  currentState: EditableResourceSessionState | null,
  selectedEditableValues: ReturnType<typeof resolveEditableResourceValues>,
  selectionKey: string | null,
): EditableResourceSessionState | null {
  if (!selectionKey || !selectedEditableValues) {
    return null;
  }

  const initialDraft = editableResourceDraftFromValues(selectedEditableValues);
  if (!currentState || currentState.selectionKey !== selectionKey) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  if (
    areEditableResourceDraftsEqual(
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

  return areEditableResourceDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
