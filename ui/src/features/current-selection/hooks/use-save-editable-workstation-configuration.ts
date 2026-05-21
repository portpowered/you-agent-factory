import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";

import { normalizeFactoryDefinition } from "../../../api/factory-definition";
import {
  createFactory,
  type FactoryValue,
  NamedFactoryAPIError,
} from "../../../api/named-factory";
import { CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY } from "../../current-factory-definition";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationSaveState,
} from "../detail-card-types";
import { getWorkstationDetailMessages } from "../messages";

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
  markChangesSaved?: () => void;
  scopeKey: string;
  value: FactoryValue;
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
  const queryClient = useQueryClient();
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
  const mutation = useMutation({
    mutationFn: ({ value }: EditableWorkstationSaveRequest) =>
      createFactory(value),
    onError: (error, variables) => {
      setIsConfirming(false);
      setSubmittingScopeKey(null);
      setLastSuccessfulScopeKey(null);
      setLastErroredScope({
        message: normalizeSaveError(
          error,
          messages.editableConfigurationSaveFallbackError,
        ),
        scopeKey: variables.scopeKey,
      });
    },
    onSuccess: (value, variables) => {
      const normalizedFactory = normalizeFactoryDefinition(value);

      queryClient.setQueryData(
        CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY,
        normalizedFactory,
      );
      variables.markChangesSaved?.();
      setIsConfirming(false);
      setSubmittingScopeKey(null);
      setLastErroredScope(null);
      setLastSuccessfulScopeKey(variables.scopeKey);
    },
  });

  useResetExitedSaveScope({
    scopeKey,
    setIsConfirming,
    setLastErroredScope,
    setLastSuccessfulScopeKey,
  });

  useEffect(() => {
    if (
      editableConfigurationState?.status === "ready" &&
      editableConfigurationState.isDirty
    ) {
      setLastSuccessfulScopeKey((currentScopeKey) =>
        currentScopeKey === scopeKey ? null : currentScopeKey,
      );
    }
  }, [editableConfigurationState, scopeKey]);

  const canSave =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty &&
    !editableConfigurationState.hasValidationErrors &&
    editableConfigurationState.pendingFactoryDefinition != null &&
    !mutation.isPending;

  const saveState = useMemo(
    () =>
      resolveEditableWorkstationSaveState({
        isConfirming,
        lastErroredScope,
        lastSuccessfulScopeKey,
        scopeKey,
        submittingScopeKey,
      }),
    [
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
        scopeKey == null
      ) {
        return;
      }

      setLastErroredScope(null);
      setLastSuccessfulScopeKey(null);
      setSubmittingScopeKey(scopeKey);
      try {
        await mutation.mutateAsync(
          {
            markChangesSaved: editableConfigurationState.markChangesSaved,
            scopeKey,
            value: editableConfigurationState.pendingFactoryDefinition,
          },
        );
      } catch {
        return;
      }
    },
    saveState,
  };
}

function resolveEditableWorkstationSaveState({
  isConfirming,
  lastErroredScope,
  lastSuccessfulScopeKey,
  scopeKey,
  submittingScopeKey,
}: {
  isConfirming: boolean;
  lastErroredScope: EditableWorkstationErrorState | null;
  lastSuccessfulScopeKey: string | null;
  scopeKey: string | null;
  submittingScopeKey: string | null;
}): EditableWorkstationSaveState {
  if (submittingScopeKey !== null && submittingScopeKey === scopeKey) {
    return { status: "submitting" };
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
  useEffect(() => {
    if (scopeKey == null) {
      setIsConfirming(false);
      setLastErroredScope(null);
      setLastSuccessfulScopeKey(null);
      return;
    }
    setIsConfirming(false);
  }, [
    scopeKey,
    setIsConfirming,
    setLastErroredScope,
    setLastSuccessfulScopeKey,
  ]);
}

function normalizeSaveError(error: unknown, fallbackMessage: string): string {
  if (error instanceof NamedFactoryAPIError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }

  return fallbackMessage;
}
