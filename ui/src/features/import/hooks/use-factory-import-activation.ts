import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  activateImportedFactoryDocumentForSession,
  activateImportedFactoryForSession,
  type ImportFactoryValue,
  SessionFactoryAPIError,
} from "../../../api/session-factory";
import { syncCurrentFactoryDocumentCache } from "../../current-factory-definition/lib/sync-current-factory-document-cache";
import type { FactoryImportConfirmInput } from "../lib/factory-import-save-choice";

export type FactoryImportActivationState =
  | { status: "idle" }
  | { status: "submitting" }
  | { error: SessionFactoryAPIError; status: "error" };

export interface UseFactoryImportActivationOptions {
  activateFactory?: (
    input: FactoryImportConfirmInput,
  ) => Promise<ImportFactoryValue>;
  currentFactoryDocument?: CurrentFactoryDocument | null;
  onActivated?: (value: ImportFactoryValue) => void;
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
  currentFactoryDocument,
  onActivated,
  sessionID,
}: UseFactoryImportActivationOptions = {}): UseFactoryImportActivationResult {
  const queryClient = useQueryClient();
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
  const [activationError, setActivationError] =
    useState<SessionFactoryAPIError | null>(null);
  const mutation = useMutation({
    mutationFn: async (input: FactoryImportConfirmInput) => {
      if (activateFactoryOverride || !currentFactoryDocument) {
        return {
          activatedFactory: await activateFactory(input),
          savedDocument: null,
        };
      }

      const savedDocument = await activateImportedFactoryDocumentForSession(
        input.value.factory,
        {
          choice: input.choice,
          createFactoryName: input.createFactoryName,
          currentDocument: currentFactoryDocument,
          existingFactoryNames: input.existingFactoryNames,
          sessionID,
        },
      );

      return {
        activatedFactory: currentFactoryDocumentToImportValue(savedDocument),
        savedDocument,
      };
    },
    onError: (error) => {
      setActivationError(normalizeActivationError(error));
    },
    onSuccess: async ({ activatedFactory, savedDocument }) => {
      setActivationError(null);
      if (savedDocument) {
        syncCurrentFactoryDocumentCache(queryClient, sessionID, savedDocument);
      }
      onActivated?.(activatedFactory);
    },
  });

  const activateImport = useCallback(
    async (input: FactoryImportConfirmInput) => {
      setActivationError(null);
      try {
        await mutation.mutateAsync(input);
      } catch {
        return;
      }
    },
    [mutation],
  );

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

function currentFactoryDocumentToImportValue(
  document: CurrentFactoryDocument,
): ImportFactoryValue {
  const { version: _version, ...factoryValue } = document;
  return factoryValue;
}

function normalizeActivationError(error: unknown): SessionFactoryAPIError {
  if (error instanceof SessionFactoryAPIError) {
    return error;
  }

  if (error instanceof Error) {
    return new SessionFactoryAPIError(error.message, {
      code: "INTERNAL_ERROR",
    });
  }

  return new SessionFactoryAPIError("Factory activation failed.", {
    code: "INTERNAL_ERROR",
  });
}
