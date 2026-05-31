import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDefinitionError,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import { useSaveCurrentFactory } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationSaveState,
  EditableWorkstationSaveValidationErrors,
} from "../lib/detail-card-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";

interface UseSaveEditableWorkstationConfigurationOptions {
  editableConfigurationState?: EditableWorkstationConfigurationState;
  locale?: string | null;
  scopeKey: string | null;
}

interface UseSaveEditableWorkstationConfigurationResult {
  beginSaveConfirmation: () => void;
  canSave: boolean;
  cancelSaveConfirmation: () => void;
  confirmSave: () => Promise<void>;
  saveState: EditableWorkstationSaveState;
}

interface EditableWorkstationSaveRequest {
  baseVersion: CurrentFactoryVersion;
  markChangesSaved?: () => void;
  scopeKey: string;
  value: CanonicalFactoryDefinition;
}

interface EditableWorkstationErrorState {
  fieldErrors?: EditableWorkstationSaveValidationErrors;
  message: string;
  scopeKey: string;
  status: "error" | "warning";
}

export function useSaveEditableWorkstationConfiguration({
  editableConfigurationState,
  locale,
  scopeKey,
}: UseSaveEditableWorkstationConfigurationOptions): UseSaveEditableWorkstationConfigurationResult {
  const messages = getWorkstationDetailMessages(locale);
  const [isConfirming, setIsConfirming] = useState(false);
  const [lastFailedScope, setLastFailedScope] =
    useState<EditableWorkstationErrorState | null>(null);
  const [lastSuccessfulScopeKey, setLastSuccessfulScopeKey] = useState<
    string | null
  >(null);
  const [submittingScopeKey, setSubmittingScopeKey] = useState<string | null>(
    null,
  );
  const saveInFlightRef = useRef(false);
  const mutation = useSaveCurrentFactory();

  useResetExitedSaveScope({
    scopeKey,
    setIsConfirming,
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
    editableConfigurationState.isDirty &&
    !editableConfigurationState.hasValidationErrors &&
    editableConfigurationState.pendingFactoryDefinition != null &&
    !mutation.isPending;

  const beginSaveConfirmation = useCallback(() => {
    setLastSuccessfulScopeKey(null);
    setLastFailedScope(null);
    setIsConfirming(true);
  }, []);

  const saveState = useMemo(
    () =>
      resolveEditableWorkstationSaveState({
        hasScopeChanged,
        isConfirming,
        lastFailedScope,
        lastSuccessfulScopeKey,
        scopeKey,
        submittingScopeKey,
      }),
    [
      hasScopeChanged,
      isConfirming,
      lastFailedScope,
      lastSuccessfulScopeKey,
      scopeKey,
      submittingScopeKey,
    ],
  );

  return {
    beginSaveConfirmation,
    canSave,
    cancelSaveConfirmation: () => {
      if (!mutation.isPending) {
        setIsConfirming(false);
      }
    },
    confirmSave: async () => {
      if (
        editableConfigurationState?.status !== "ready" ||
        editableConfigurationState.pendingFactoryDefinition == null ||
        scopeKey == null ||
        saveInFlightRef.current
      ) {
        return;
      }

      setLastFailedScope(null);
      setLastSuccessfulScopeKey(null);
      saveInFlightRef.current = true;
      setSubmittingScopeKey(scopeKey);
      const request: EditableWorkstationSaveRequest = {
        baseVersion: editableConfigurationState.baseVersion,
        markChangesSaved: editableConfigurationState.markChangesSaved,
        scopeKey,
        value: editableConfigurationState.pendingFactoryDefinition,
      };
      try {
        await mutation.mutateAsync({
          baseVersion: request.baseVersion,
          factoryDefinition: request.value,
        });
        request.markChangesSaved?.();
        setIsConfirming(false);
        setSubmittingScopeKey(null);
        setLastFailedScope(null);
        setLastSuccessfulScopeKey(request.scopeKey);
      } catch (error) {
        setIsConfirming(false);
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
    },
    saveState,
  };
}

function resolveEditableWorkstationSaveState({
  hasScopeChanged,
  isConfirming,
  lastFailedScope,
  lastSuccessfulScopeKey,
  scopeKey,
  submittingScopeKey,
}: {
  hasScopeChanged: boolean;
  isConfirming: boolean;
  lastFailedScope: EditableWorkstationErrorState | null;
  lastSuccessfulScopeKey: string | null;
  scopeKey: string | null;
  submittingScopeKey: string | null;
}): EditableWorkstationSaveState {
  if (submittingScopeKey !== null && submittingScopeKey === scopeKey) {
    return { status: "submitting" };
  }
  if (hasScopeChanged) {
    return { status: "idle" };
  }
  if (isConfirming) {
    return { status: "confirming" };
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
  setIsConfirming,
  setLastFailedScope,
  setLastSuccessfulScopeKey,
}: {
  scopeKey: string | null;
  setIsConfirming: (value: boolean) => void;
  setLastFailedScope: (value: EditableWorkstationErrorState | null) => void;
  setLastSuccessfulScopeKey: (value: string | null) => void;
}) {
  const previousScopeKeyRef = useRef<string | null>(scopeKey);

  useEffect(() => {
    if (previousScopeKeyRef.current !== scopeKey) {
      setIsConfirming(false);
      setLastFailedScope(null);
      setLastSuccessfulScopeKey(null);
      previousScopeKeyRef.current = scopeKey;
    }
  }, [
    scopeKey,
    setIsConfirming,
    setLastFailedScope,
    setLastSuccessfulScopeKey,
  ]);
}

function useResetSuccessfulSaveStateOnDraftChange({
  editableConfigurationState,
  scopeKey,
  setLastSuccessfulScopeKey,
}: {
  editableConfigurationState?: EditableWorkstationConfigurationState;
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
): Pick<EditableWorkstationErrorState, "fieldErrors" | "message" | "status"> {
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
): EditableWorkstationSaveValidationErrors | undefined {
  const fieldErrors: EditableWorkstationSaveValidationErrors = {};

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
): keyof EditableWorkstationSaveValidationErrors | null {
  const subjectID = target.subject.id.trim().toLowerCase();
  const subjectType = target.subject.type;
  const subjectLocation = target.subject.location;

  if (
    target.code === "factory.worker.danglingReference" &&
    subjectType === "WORKSTATION"
  ) {
    return "workerName";
  }
  if (
    subjectType === "WORKSTATION" &&
    (subjectLocation === "REFERENCE" || subjectLocation === "DEFINITION")
  ) {
    if (subjectID.endsWith("worker") || subjectID === "worker") {
      return "workerName";
    }
    if (subjectID === "behavior") {
      return "behavior";
    }
    if (subjectID === "body" || subjectID === "prompt") {
      return "prompt";
    }
    if (subjectID === "runner" || subjectID === "runnername") {
      return "runnerName";
    }
  }

  return null;
}
