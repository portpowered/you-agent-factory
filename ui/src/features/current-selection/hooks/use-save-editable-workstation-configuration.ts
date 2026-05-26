import { useEffect, useMemo, useRef, useState } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../api/current-factory-definition";
import {
  useSaveCurrentFactory,
} from "../../current-factory-definition/public";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationSaveState,
} from "../components/detail-card-types";
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
  message: string;
  scopeKey: string;
}

export function useSaveEditableWorkstationConfiguration({
  editableConfigurationState,
  locale,
  scopeKey,
}: UseSaveEditableWorkstationConfigurationOptions): UseSaveEditableWorkstationConfigurationResult {
  const messages = getWorkstationDetailMessages(locale);
  const [isConfirming, setIsConfirming] = useState(false);
  const [lastErroredScope, setLastErroredScope] =
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
    setLastErroredScope,
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

  const saveState = useMemo(
    () =>
      resolveEditableWorkstationSaveState({
        hasScopeChanged,
        isConfirming,
        lastErroredScope,
        lastSuccessfulScopeKey,
        scopeKey,
        submittingScopeKey,
      }),
    [
      hasScopeChanged,
      isConfirming,
      lastErroredScope,
      lastSuccessfulScopeKey,
      scopeKey,
      submittingScopeKey,
    ],
  );

  return {
    beginSaveConfirmation: () => {
      if (!canSave) {
        return;
      }
      setLastSuccessfulScopeKey(null);
      setLastErroredScope(null);
      setIsConfirming(true);
    },
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

      setLastErroredScope(null);
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
        setLastErroredScope(null);
        setLastSuccessfulScopeKey(request.scopeKey);
      } catch (error) {
        setIsConfirming(false);
        setSubmittingScopeKey(null);
        setLastSuccessfulScopeKey(null);
        setLastErroredScope({
          message: normalizeSaveError(
            error,
            messages.editableConfigurationSaveFallbackError,
          ),
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
  lastErroredScope,
  lastSuccessfulScopeKey,
  scopeKey,
  submittingScopeKey,
}: {
  hasScopeChanged: boolean;
  isConfirming: boolean;
  lastErroredScope: EditableWorkstationErrorState | null;
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
    lastErroredScope !== null &&
    scopeKey !== null &&
    lastErroredScope.scopeKey === scopeKey
  ) {
    return {
      errorMessage: lastErroredScope.message,
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
  setLastErroredScope,
  setLastSuccessfulScopeKey,
}: {
  scopeKey: string | null;
  setIsConfirming: (value: boolean) => void;
  setLastErroredScope: (value: EditableWorkstationErrorState | null) => void;
  setLastSuccessfulScopeKey: (value: string | null) => void;
}) {
  const previousScopeKeyRef = useRef<string | null>(scopeKey);

  useEffect(() => {
    if (previousScopeKeyRef.current !== scopeKey) {
      setIsConfirming(false);
      setLastErroredScope(null);
      setLastSuccessfulScopeKey(null);
      previousScopeKeyRef.current = scopeKey;
    }
  }, [
    scopeKey,
    setIsConfirming,
    setLastErroredScope,
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

function normalizeSaveError(error: unknown, fallbackMessage: string): string {
  if (error instanceof Error) {
    return error.message;
  }

  return fallbackMessage;
}
