import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";

import { normalizeFactoryDefinition } from "../../api/factory-definition";
import {
  createFactory,
  type FactoryValue,
  NamedFactoryAPIError,
} from "../../api/named-factory";
import { CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY } from "../current-factory-definition";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationSaveState,
} from "./detail-card-types";

interface UseSaveEditableWorkstationConfigurationOptions {
  editableConfigurationState?: EditableWorkstationConfigurationState;
  scopeKey: string | null;
}

interface UseSaveEditableWorkstationConfigurationResult {
  beginSaveConfirmation: () => void;
  canSave: boolean;
  cancelSaveConfirmation: () => void;
  confirmSave: () => Promise<void>;
  saveState: EditableWorkstationSaveState;
}

export function useSaveEditableWorkstationConfiguration({
  editableConfigurationState,
  scopeKey,
}: UseSaveEditableWorkstationConfigurationOptions): UseSaveEditableWorkstationConfigurationResult {
  const queryClient = useQueryClient();
  const [isConfirming, setIsConfirming] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [isSuccess, setIsSuccess] = useState(false);
  const mutation = useMutation({
    mutationFn: (value: FactoryValue) => createFactory(value),
    onError: (error) => {
      setIsConfirming(false);
      setIsSuccess(false);
      setSaveError(normalizeSaveError(error));
    },
    onSuccess: (value) => {
      const normalizedFactory = normalizeFactoryDefinition(value);

      queryClient.setQueryData(
        CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY,
        normalizedFactory,
      );
      if (editableConfigurationState?.status === "ready") {
        editableConfigurationState.markChangesSaved();
      }
      setIsConfirming(false);
      setSaveError(null);
      setIsSuccess(true);
    },
  });

  useEffect(() => {
    if (scopeKey === null) {
      setIsConfirming(false);
      setIsSuccess(false);
      setSaveError(null);
      return;
    }

    setIsConfirming(false);
    setIsSuccess(false);
    setSaveError(null);
  }, [scopeKey]);

  useEffect(() => {
    if (
      editableConfigurationState?.status === "ready" &&
      editableConfigurationState.isDirty
    ) {
      setIsSuccess(false);
    }
  }, [editableConfigurationState]);

  const canSave =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty &&
    !editableConfigurationState.hasValidationErrors &&
    editableConfigurationState.pendingFactoryDefinition != null &&
    !mutation.isPending;

  const saveState = useMemo<EditableWorkstationSaveState>(() => {
    if (mutation.isPending) {
      return { status: "submitting" };
    }
    if (isConfirming) {
      return { status: "confirming" };
    }
    if (saveError) {
      return {
        errorMessage: saveError,
        status: "error",
      };
    }
    if (isSuccess) {
      return { status: "success" };
    }
    return { status: "idle" };
  }, [isConfirming, isSuccess, mutation.isPending, saveError]);

  return {
    beginSaveConfirmation: () => {
      if (!canSave) {
        return;
      }
      setIsSuccess(false);
      setSaveError(null);
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
        editableConfigurationState.pendingFactoryDefinition == null
      ) {
        return;
      }

      setSaveError(null);
      setIsSuccess(false);
      try {
        await mutation.mutateAsync(
          editableConfigurationState.pendingFactoryDefinition,
        );
      } catch {
        return;
      }
    },
    saveState,
  };
}

function normalizeSaveError(error: unknown): string {
  if (error instanceof NamedFactoryAPIError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }

  return "The running factory could not be saved.";
}
