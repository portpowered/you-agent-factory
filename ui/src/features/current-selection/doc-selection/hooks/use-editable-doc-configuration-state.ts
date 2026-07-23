import { useEffect, useMemo, useState } from "react";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import {
  applyEditableDocDraft,
  type EditableDocDraft,
  editableDocDraftFromValues,
  resolveEditableDocValues,
} from "../../../current-factory-definition/lib/doc-editable-values";
import type { DashboardSelection } from "../../base/state/selection-types";
import { resolveEditableDocOverwriteFields } from "../editing/editable-doc-overwrite-fields";
import type { EditableDocConfigurationState } from "../lib/detail-card-types";
import {
  hasEditableDocValidationErrors,
  listEditableDocTargetPaths,
  mergeEditableDocContractValidationErrors,
  resolvePendingDocTargetPath,
  validateEditableDocDraft,
} from "../lib/doc-editable-validation";
import { getDocDetailMessages } from "../messages/doc-detail";

interface EditableDocSessionState {
  draft: EditableDocDraft;
  latestDefinitionDraft: EditableDocDraft;
  selectionKey: string;
  sessionStartDraft: EditableDocDraft;
}

export function useEditableDocConfigurationState(
  selection: DashboardSelection | null,
  targetPath: string | null,
  locale?: string | null,
  editableDefinition?: CurrentFactoryDocument | null,
): EditableDocConfigurationState | undefined {
  const isDocSelection = selection?.kind === "doc" && targetPath != null;
  const messages = getDocDetailMessages(locale);
  const { selectedEditableValues, sessionState, setSessionState } =
    useEditableDocSession(editableDefinition, targetPath, isDocSelection);

  if (!isDocSelection || !targetPath) {
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

  return buildReadyEditableDocConfigurationState({
    editableDefinition,
    sessionState,
    setSessionState,
    selectedEditableValues,
    targetPath,
  });
}

function useEditableDocSession(
  editableDefinition: CurrentFactoryDocument | null | undefined,
  targetPath: string | null,
  isDocSelection: boolean,
) {
  const selectedEditableValues = useMemo(() => {
    if (!isDocSelection || !targetPath || !editableDefinition) {
      return null;
    }

    return resolveEditableDocValues(editableDefinition, targetPath);
  }, [editableDefinition, isDocSelection, targetPath]);
  const selectionKey = isDocSelection && targetPath ? targetPath : null;
  const [sessionState, setSessionState] =
    useState<EditableDocSessionState | null>(null);

  useEffect(() => {
    setSessionState((currentState) =>
      syncEditableDocSession(
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

function buildReadyEditableDocConfigurationState({
  editableDefinition,
  sessionState,
  setSessionState,
  selectedEditableValues,
  targetPath,
}: {
  editableDefinition: NonNullable<CurrentFactoryDocument>;
  sessionState: EditableDocSessionState;
  setSessionState: (
    updater: (
      currentState: EditableDocSessionState | null,
    ) => EditableDocSessionState | null,
  ) => void;
  selectedEditableValues: NonNullable<
    ReturnType<typeof resolveEditableDocValues>
  >;
  targetPath: string;
}): Extract<EditableDocConfigurationState, { status: "ready" }> {
  const pendingFactoryDefinition = applyEditableDocDraft(
    editableDefinition,
    targetPath,
    sessionState.draft,
  );
  const messages = getDocDetailMessages();
  const validationErrors = mergeEditableDocContractValidationErrors(
    validateEditableDocDraft(sessionState.draft, messages, {
      docTargetPaths: listEditableDocTargetPaths(editableDefinition),
      originalTargetPath: targetPath,
    }),
    pendingFactoryDefinition,
  );
  const hasValidationErrors = hasEditableDocValidationErrors(validationErrors);
  const isDirty = !areEditableDocDraftsEqual(
    sessionState.draft,
    sessionState.sessionStartDraft,
  );
  const pendingTargetPath = resolvePendingDocTargetPath(sessionState.draft);

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
    onFileNameChange: (value) => {
      updateDraft(setSessionState, (draft) => ({ ...draft, fileName: value }));
    },
    onInlineContentChange: (value) => {
      updateDraft(setSessionState, (draft) => ({
        ...draft,
        inlineContent: value,
      }));
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
    originalTargetPath: targetPath,
    overwriteFieldNames: resolveEditableDocOverwriteFields(
      sessionState.sessionStartDraft,
      sessionState.draft,
      sessionState.latestDefinitionDraft,
    ),
    pendingFactoryDefinition,
    pendingTargetPath,
    savedFactoryDefinition: editableDefinition,
    status: "ready",
    validationErrors,
  };
}

function updateDraft(
  setSessionState: (
    updater: (
      currentState: EditableDocSessionState | null,
    ) => EditableDocSessionState | null,
  ) => void,
  updater: (draft: EditableDocDraft) => EditableDocDraft,
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

function areEditableDocDraftsEqual(
  left: EditableDocDraft,
  right: EditableDocDraft,
): boolean {
  return (
    left.fileName === right.fileName &&
    left.inlineContent === right.inlineContent
  );
}

function syncEditableDocSession(
  currentState: EditableDocSessionState | null,
  selectedEditableValues: ReturnType<typeof resolveEditableDocValues>,
  selectionKey: string | null,
): EditableDocSessionState | null {
  if (!selectionKey || !selectedEditableValues) {
    return null;
  }

  const initialDraft = editableDocDraftFromValues(selectedEditableValues);
  if (!currentState || currentState.selectionKey !== selectionKey) {
    return {
      draft: initialDraft,
      latestDefinitionDraft: initialDraft,
      selectionKey,
      sessionStartDraft: initialDraft,
    };
  }

  if (
    areEditableDocDraftsEqual(
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

  return areEditableDocDraftsEqual(
    currentState.latestDefinitionDraft,
    initialDraft,
  )
    ? currentState
    : {
        ...currentState,
        latestDefinitionDraft: initialDraft,
      };
}
