import { useMutation } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";

import {
  createFactory,
  type FactoryValue,
  NamedFactoryAPIError,
  type NamedFactoryAPIErrorCode,
} from "../../api/named-factory";
import type { FactoryPngImportValue } from "./factory-png-import";

export type FactoryImportActivationErrorCode = NamedFactoryAPIErrorCode;

export type FactoryImportActivationState =
  | { status: "idle" }
  | { status: "submitting" }
  | { error: NamedFactoryAPIError; status: "error" };

export interface UseFactoryImportActivationOptions {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  onActivated?: (value: FactoryValue) => void;
}

export interface UseFactoryImportActivationResult {
  activateImport: (value: FactoryPngImportValue) => Promise<void>;
  activationState: FactoryImportActivationState;
  clearActivationError: () => void;
}

const IDLE_ACTIVATION_STATE: FactoryImportActivationState = { status: "idle" };

export function useFactoryImportActivation({
  activateFactory = createFactory,
  onActivated,
}: UseFactoryImportActivationOptions = {}): UseFactoryImportActivationResult {
  const [activationError, setActivationError] = useState<NamedFactoryAPIError | null>(null);
  const mutation = useMutation({
    mutationFn: (value: FactoryValue) => activateFactory(value),
    onError: (error) => {
      setActivationError(normalizeActivationError(error));
    },
    onSuccess: (value) => {
      setActivationError(null);
      onActivated?.(value);
    },
  });

  const activateImport = useCallback(async (value: FactoryPngImportValue) => {
    setActivationError(null);
    try {
      await mutation.mutateAsync(value.factory);
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

