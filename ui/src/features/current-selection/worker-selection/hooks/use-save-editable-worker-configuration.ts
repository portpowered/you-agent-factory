import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDefinitionError,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import { useFactoryDocumentSave } from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import type {
  EditableWorkerConfigurationState,
  EditableWorkerSaveState,
  EditableWorkerSaveValidationErrors,
} from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";

interface UseSaveEditableWorkerConfigurationOptions {
  editableConfigurationState?: EditableWorkerConfigurationState;
  locale?: string | null;
  scopeKey: string | null;
}

interface UseSaveEditableWorkerConfigurationResult {
  canSave: boolean;
  save: () => Promise<void>;
  saveState: EditableWorkerSaveState;
}

interface EditableWorkerSaveRequest {
  baseVersion: CurrentFactoryVersion;
  markChangesSaved?: () => void;
  scopeKey: string;
  value: CanonicalFactoryDefinition;
}

interface EditableWorkerErrorState {
  fieldErrors?: EditableWorkerSaveValidationErrors;
  message: string;
  scopeKey: string;
  status: "error" | "warning";
}

export function useSaveEditableWorkerConfiguration({
  editableConfigurationState,
  locale,
  scopeKey,
}: UseSaveEditableWorkerConfigurationOptions): UseSaveEditableWorkerConfigurationResult {
  const messages = getWorkerDetailMessages(locale);
  const [lastFailedScope, setLastFailedScope] =
    useState<EditableWorkerErrorState | null>(null);
  const [lastSuccessfulScopeKey, setLastSuccessfulScopeKey] = useState<
    string | null
  >(null);
  const [submittingScopeKey, setSubmittingScopeKey] = useState<string | null>(
    null,
  );
  const saveInFlightRef = useRef(false);
  const { isPending, saveAsync } = useFactoryDocumentSave();

  useResetExitedSaveScope({
    scopeKey,
    setLastFailedScope,
    setLastSuccessfulScopeKey,
  });
  const previousScopeKeyRef = useRef<string | null>(scopeKey);
  const hasScopeChanged = previousScopeKeyRef.current !== scopeKey;
  useResetSuccessfulSaveStateOnDraftChange({
    editableConfigurationState,
    scopeKey,
    setLastSuccessfulScopeKey,
  });

  const canSave =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.canSave &&
    editableConfigurationState.pendingFactoryDefinition != null &&
    !isPending;

  const saveState = useMemo(
    () =>
      resolveEditableWorkerSaveState({
        hasScopeChanged,
        lastFailedScope,
        lastSuccessfulScopeKey,
        scopeKey,
        submittingScopeKey,
      }),
    [
      hasScopeChanged,
      lastFailedScope,
      lastSuccessfulScopeKey,
      scopeKey,
      submittingScopeKey,
    ],
  );

  const save = useCallback(async () => {
    if (
      editableConfigurationState?.status !== "ready" ||
      editableConfigurationState.pendingFactoryDefinition == null ||
      editableConfigurationState.baseVersion == null ||
      scopeKey == null ||
      saveInFlightRef.current
    ) {
      return;
    }

    setLastFailedScope(null);
    setLastSuccessfulScopeKey(null);
    saveInFlightRef.current = true;
    setSubmittingScopeKey(scopeKey);
    const request: EditableWorkerSaveRequest = {
      baseVersion: editableConfigurationState.baseVersion,
      markChangesSaved: editableConfigurationState.markChangesSaved,
      scopeKey,
      value: editableConfigurationState.pendingFactoryDefinition,
    };
    try {
      await saveAsync({
        baseVersion: request.baseVersion,
        factory: request.value,
      });
      request.markChangesSaved?.();
      setSubmittingScopeKey(null);
      setLastFailedScope(null);
      setLastSuccessfulScopeKey(request.scopeKey);
    } catch (error) {
      setSubmittingScopeKey(null);
      setLastSuccessfulScopeKey(null);
      setLastFailedScope({
        ...normalizeSaveError(error, {
          fallbackMessage: messages.editableConfigurationSaveFallbackError,
        }),
        scopeKey: request.scopeKey,
      });
      return;
    } finally {
      saveInFlightRef.current = false;
    }
  }, [
    editableConfigurationState,
    messages.editableConfigurationSaveFallbackError,
    saveAsync,
    scopeKey,
  ]);

  return {
    canSave,
    save,
    saveState,
  };
}

function resolveEditableWorkerSaveState({
  hasScopeChanged,
  lastFailedScope,
  lastSuccessfulScopeKey,
  scopeKey,
  submittingScopeKey,
}: {
  hasScopeChanged: boolean;
  lastFailedScope: EditableWorkerErrorState | null;
  lastSuccessfulScopeKey: string | null;
  scopeKey: string | null;
  submittingScopeKey: string | null;
}): EditableWorkerSaveState {
  if (submittingScopeKey !== null && submittingScopeKey === scopeKey) {
    return { status: "submitting" };
  }
  if (hasScopeChanged) {
    return { status: "idle" };
  }
  if (
    lastFailedScope !== null &&
    scopeKey !== null &&
    lastFailedScope.scopeKey === scopeKey
  ) {
    if (lastFailedScope.status === "warning") {
      return {
        message: lastFailedScope.message,
        status: "warning",
      };
    }

    return {
      errorMessage: lastFailedScope.message,
      fieldErrors: lastFailedScope.fieldErrors,
      status: "error",
    };
  }
  if (lastSuccessfulScopeKey !== null && lastSuccessfulScopeKey === scopeKey) {
    return { status: "success" };
  }

  return { status: "idle" };
}

function useResetExitedSaveScope({
  scopeKey,
  setLastFailedScope,
  setLastSuccessfulScopeKey,
}: {
  scopeKey: string | null;
  setLastFailedScope: (value: EditableWorkerErrorState | null) => void;
  setLastSuccessfulScopeKey: (value: string | null) => void;
}) {
  const previousScopeKeyRef = useRef<string | null>(scopeKey);

  useEffect(() => {
    if (previousScopeKeyRef.current !== scopeKey) {
      setLastFailedScope(null);
      setLastSuccessfulScopeKey(null);
      previousScopeKeyRef.current = scopeKey;
    }
  }, [scopeKey, setLastFailedScope, setLastSuccessfulScopeKey]);
}

function useResetSuccessfulSaveStateOnDraftChange({
  editableConfigurationState,
  scopeKey,
  setLastSuccessfulScopeKey,
}: {
  editableConfigurationState?: EditableWorkerConfigurationState;
  scopeKey: string | null;
  setLastSuccessfulScopeKey: (
    value: string | null | ((currentScopeKey: string | null) => string | null),
  ) => void;
}) {
  useEffect(() => {
    if (
      editableConfigurationState?.status === "ready" &&
      editableConfigurationState.isDirty
    ) {
      setLastSuccessfulScopeKey((currentScopeKey) =>
        currentScopeKey === scopeKey ? null : currentScopeKey,
      );
    }
  }, [editableConfigurationState, scopeKey, setLastSuccessfulScopeKey]);
}

function normalizeSaveError(
  error: unknown,
  {
    fallbackMessage,
  }: {
    fallbackMessage: string;
  },
): Pick<EditableWorkerErrorState, "fieldErrors" | "message" | "status"> {
  if (!isCurrentFactoryDefinitionError(error)) {
    if (error instanceof Error) {
      return {
        message: error.message,
        status: "error",
      };
    }

    return {
      message: fallbackMessage,
      status: "error",
    };
  }

  if (error.code === "STALE_FACTORY_VERSION") {
    return {
      message: error.message,
      status: "warning",
    };
  }

  return {
    fieldErrors: resolveSaveFieldErrors(error),
    message: error.message,
    status: "error",
  };
}

function isCurrentFactoryDefinitionError(
  error: unknown,
): error is Pick<
  CurrentFactoryDefinitionError,
  "code" | "message" | "targets"
> {
  return (
    typeof error === "object" &&
    error !== null &&
    typeof (error as { code?: unknown }).code === "string" &&
    typeof (error as { message?: unknown }).message === "string"
  );
}

function resolveSaveFieldErrors(
  error: Pick<CurrentFactoryDefinitionError, "message" | "targets">,
): EditableWorkerSaveValidationErrors | undefined {
  const fieldErrors: EditableWorkerSaveValidationErrors = {};

  for (const target of error.targets ?? []) {
    const fieldName = resolveTargetFieldName(target);
    if (fieldName === null) {
      continue;
    }
    fieldErrors[fieldName] ??= error.message;
  }

  return Object.keys(fieldErrors).length > 0 ? fieldErrors : undefined;
}

function resolveTargetFieldName(
  target: NonNullable<CurrentFactoryDefinitionError["targets"]>[number],
): keyof EditableWorkerSaveValidationErrors | null {
  if (target.subject.type !== "WORKER") {
    return null;
  }

  const subjectID = target.subject.id.trim().toLowerCase();

  if (subjectID === "type") {
    return "type";
  }
  if (subjectID === "modelprovider") {
    return "modelProvider";
  }
  if (subjectID === "model") {
    return "model";
  }
  if (subjectID === "modellocality") {
    return "modelLocality";
  }
  if (subjectID === "executorprovider") {
    return "executorProvider";
  }
  if (subjectID === "command") {
    return "command";
  }
  if (subjectID === "args") {
    return "args";
  }
  if (subjectID === "body") {
    return "body";
  }
  if (subjectID === "provider") {
    return "provider";
  }

  return null;
}
