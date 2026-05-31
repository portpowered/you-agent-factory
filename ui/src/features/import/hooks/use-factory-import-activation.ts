import { useMutation } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";

import {
  activateImportedFactoryForSession,
  SessionFactoryAPIError,
  type FactoryValue,
} from "../../../api/session-factory";
import type { FactoryImportConfirmInput } from "../lib/factory-import-save-choice";

export type FactoryImportActivationState =
  | { status: "idle" }
  | { status: "submitting" }
  | { error: SessionFactoryAPIError; status: "error" };

export interface UseFactoryImportActivationOptions {
  activateFactory?: (input: FactoryImportConfirmInput) => Promise<FactoryValue>;
  onActivated?: (value: FactoryValue) => void;
  sessionID?: string | null;
}

export interface UseFactoryImportActivationResult {
  activateImport: (input: FactoryImportConfirmInput) => Promise<void>;
  activationState: FactoryImportActivationState;
  clearActivationError: () => void;
}

const IDLE_ACTIVATION_STATE: FactoryImportActivationState = { status: "idle" };

export function useFactoryImportActivation({
  activateFactory: activateFactoryOverride,
  onActivated,
  sessionID,
}: UseFactoryImportActivationOptions = {}): UseFactoryImportActivationResult {
  const activateFactory = useMemo(
    () =>
      activateFactoryOverride ??
      ((input: FactoryImportConfirmInput) =>
        activateImportedFactoryForSession(input.value.factory, {
          choice: input.choice,
          createFactoryName: input.createFactoryName,
          existingFactoryNames: input.existingFactoryNames,
          sessionID,
        })),
    [activateFactoryOverride, sessionID],
  );
  const [activationError, setActivationError] = useState<SessionFactoryAPIError | null>(null);
  const mutation = useMutation({
    mutationFn: (input: FactoryImportConfirmInput) => activateFactory(input),
    onError: (error) => {
      setActivationError(normalizeActivationError(error));
    },
    onSuccess: (value) => {
      setActivationError(null);
      onActivated?.(value);
    },
  });

  const activateImport = useCallback(async (input: FactoryImportConfirmInput) => {
    setActivationError(null);
    try {
      await mutation.mutateAsync(input);
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

function normalizeActivationError(error: unknown): SessionFactoryAPIError {
  if (error instanceof SessionFactoryAPIError) {
    return error;
  }

  if (error instanceof Error) {
    return new SessionFactoryAPIError(error.message, { code: "INTERNAL_ERROR" });
  }

  return new SessionFactoryAPIError("Factory activation failed.", { code: "INTERNAL_ERROR" });
}
