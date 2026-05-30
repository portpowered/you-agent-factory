import { useMutation } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";

import {
  activateImportedFactoryAsNewNamedForSession,
  activateImportedFactoryForSession,
  type FactoryImportSaveChoice,
  type FactoryValue,
  NamedFactoryAPIError,
} from "../../../api/named-factory";
import type { FactoryPngImportValue } from "../lib/factory-png-import";

export type FactoryImportActivationState =
  | { status: "idle" }
  | { status: "submitting" }
  | { error: NamedFactoryAPIError; status: "error" };

export interface UseFactoryImportActivationOptions {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  onActivated?: (value: FactoryValue) => void;
  sessionID?: string | null;
}

export interface UseFactoryImportActivationResult {
  activateImport: (
    value: FactoryPngImportValue,
    choice?: FactoryImportSaveChoice,
  ) => Promise<void>;
  activationState: FactoryImportActivationState;
  clearActivationError: () => void;
}

const IDLE_ACTIVATION_STATE: FactoryImportActivationState = { status: "idle" };

export function useFactoryImportActivation({
  activateFactory: activateFactoryOverride,
  onActivated,
  sessionID,
}: UseFactoryImportActivationOptions = {}): UseFactoryImportActivationResult {
  const [activationError, setActivationError] = useState<NamedFactoryAPIError | null>(null);
  const mutation = useMutation({
    mutationFn: ({
      choice,
      factory,
    }: {
      choice: FactoryImportSaveChoice;
      factory: FactoryValue;
    }) => {
      if (activateFactoryOverride) {
        return activateFactoryOverride(factory);
      }
      if (choice === "CREATE_NEW_NAMED") {
        return activateImportedFactoryAsNewNamedForSession(factory, { sessionID });
      }
      return activateImportedFactoryForSession(factory, { sessionID });
    },
    onError: (error) => {
      setActivationError(normalizeActivationError(error));
    },
    onSuccess: (value) => {
      setActivationError(null);
      onActivated?.(value);
    },
  });

  const activateImport = useCallback(async (
    value: FactoryPngImportValue,
    choice: FactoryImportSaveChoice = "REPLACE_CURRENT",
  ) => {
    setActivationError(null);
    try {
      await mutation.mutateAsync({ choice, factory: value.factory });
    } catch {
      return;
    }
  }, [mutation]);

  const clearActivationError = useCallback(() => {
    setActivationError(null);
  }, []);

  const activationState = useMemo<FactoryImportActivationState>(() => {
    if (mutation.isPending) {
      return { status: "submitting" };
    }
    if (activationError) {
      return { error: activationError, status: "error" };
    }
    return IDLE_ACTIVATION_STATE;
  }, [activationError, mutation.isPending]);

  return {
    activateImport,
    activationState,
    clearActivationError,
  };
}

function normalizeActivationError(error: unknown): NamedFactoryAPIError {
  if (error instanceof NamedFactoryAPIError) {
    return error;
  }

  if (error instanceof Error) {
    return new NamedFactoryAPIError(error.message, { code: "INTERNAL_ERROR" });
  }

  return new NamedFactoryAPIError("Factory activation failed.", { code: "INTERNAL_ERROR" });
}
