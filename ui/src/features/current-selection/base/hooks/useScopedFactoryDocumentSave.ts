import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDefinitionError,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import { useFactoryDocumentSave } from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import type { FactoryDocumentSaveState } from "./factory-document-save-types";

export interface ScopedFactoryDocumentSaveRequest {
  baseVersion: CurrentFactoryVersion;
  factory: CanonicalFactoryDefinition;
  onSaved?: () => void;
  scopeKey: string;
}

export interface UseScopedFactoryDocumentSaveOptions<
  TFieldErrors extends Record<string, string> = Record<string, string>,
> {
  fallbackErrorMessage: string;
  isDirty?: boolean;
  mapSaveErrorToFieldErrors?: (
    error: Pick<
      CurrentFactoryDefinitionError,
      "code" | "message" | "targets"
    >,
  ) => TFieldErrors | undefined;
  scopeKey: string | null;
}

export interface UseScopedFactoryDocumentSaveResult<
  TFieldErrors extends Record<string, string> = Record<string, string>,
> {
  beginConfirmation: () => void;
  cancelConfirmation: () => void;
  clearSaveFeedback: () => void;
  confirmSave: (request: ScopedFactoryDocumentSaveRequest) => Promise<void>;
  error: CurrentFactoryDefinitionError | null;
  isPending: boolean;
  reset: () => void;
  saveNow: (request: ScopedFactoryDocumentSaveRequest) => Promise<void>;
  saveState: FactoryDocumentSaveState<TFieldErrors>;
}

interface ScopedFactoryDocumentSaveErrorState<
  TFieldErrors extends Record<string, string>,
> {
  fieldErrors?: TFieldErrors;
  message: string;
  scopeKey: string;
  status: "error" | "warning";
}

export function useScopedFactoryDocumentSave<
  TFieldErrors extends Record<string, string> = Record<string, string>,
>({
  fallbackErrorMessage,
  isDirty = false,
  mapSaveErrorToFieldErrors,
  scopeKey,
}: UseScopedFactoryDocumentSaveOptions<TFieldErrors>): UseScopedFactoryDocumentSaveResult<TFieldErrors> {
  const [isConfirming, setIsConfirming] = useState(false);
  const [lastFailedScope, setLastFailedScope] =
    useState<ScopedFactoryDocumentSaveErrorState<TFieldErrors> | null>(null);
  const [lastSuccessfulScopeKey, setLastSuccessfulScopeKey] = useState<
    string | null
  >(null);
  const [submittingScopeKey, setSubmittingScopeKey] = useState<string | null>(
    null,
  );
  const saveInFlightRef = useRef(false);
  const activeScopeKeyRef = useRef(scopeKey);
  activeScopeKeyRef.current = scopeKey;
  const { error, isPending, reset, saveAsync } = useFactoryDocumentSave();

  useResetExitedSaveScope({
    scopeKey,
    setIsConfirming,
    setLastFailedScope,
    setLastSuccessfulScopeKey,
  });
  const previousScopeKeyRef = useRef<string | null>(scopeKey);
  const hasScopeChanged = previousScopeKeyRef.current !== scopeKey;
  if (hasScopeChanged) {
    previousScopeKeyRef.current = scopeKey;
  }
  useResetSuccessfulSaveStateOnDraftChange({
    isDirty,
    scopeKey,
    setLastSuccessfulScopeKey,
  });

  const saveState = useMemo(
    () =>
      resolveDetailCardSaveState({
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

  const persistSave = useCallback(
    (request: ScopedFactoryDocumentSaveRequest) =>
      executeScopedFactoryDocumentSave({
        activeScopeKeyRef,
        fallbackErrorMessage,
        mapSaveErrorToFieldErrors,
        request,
        saveAsync,
        saveInFlightRef,
        setIsConfirming,
        setLastFailedScope,
        setLastSuccessfulScopeKey,
        setSubmittingScopeKey,
      }),
    [
      fallbackErrorMessage,
      mapSaveErrorToFieldErrors,
      saveAsync,
    ],
  );

  const beginConfirmation = useCallback(() => {
    setLastSuccessfulScopeKey(null);
    setLastFailedScope(null);
    setIsConfirming(true);
  }, []);

  const cancelConfirmation = useCallback(() => {
    if (!isPending) {
      setIsConfirming(false);
    }
  }, [isPending]);

  const clearSaveFeedback = useCallback(() => {
    setIsConfirming(false);
    setLastFailedScope(null);
    setLastSuccessfulScopeKey(null);
  }, []);

  return {
    beginConfirmation,
    cancelConfirmation,
    clearSaveFeedback,
    confirmSave: persistSave,
    error,
    isPending,
    reset,
    saveNow: persistSave,
    saveState,
  };
}

async function executeScopedFactoryDocumentSave<
  TFieldErrors extends Record<string, string>,
>({
  activeScopeKeyRef,
  fallbackErrorMessage,
  mapSaveErrorToFieldErrors,
  request,
  saveAsync,
  saveInFlightRef,
  setIsConfirming,
  setLastFailedScope,
  setLastSuccessfulScopeKey,
  setSubmittingScopeKey,
}: {
  activeScopeKeyRef: { current: string | null };
  fallbackErrorMessage: string;
  mapSaveErrorToFieldErrors?: (
    error: Pick<
      CurrentFactoryDefinitionError,
      "code" | "message" | "targets"
    >,
  ) => TFieldErrors | undefined;
  request: ScopedFactoryDocumentSaveRequest;
  saveAsync: ReturnType<typeof useFactoryDocumentSave>["saveAsync"];
  saveInFlightRef: { current: boolean };
  setIsConfirming: (value: boolean) => void;
  setLastFailedScope: (
    value: ScopedFactoryDocumentSaveErrorState<TFieldErrors> | null,
  ) => void;
  setLastSuccessfulScopeKey: (value: string | null) => void;
  setSubmittingScopeKey: (value: string | null) => void;
}) {
  if (
    activeScopeKeyRef.current == null ||
    request.scopeKey !== activeScopeKeyRef.current ||
    saveInFlightRef.current
  ) {
    return;
  }

  setLastFailedScope(null);
  setLastSuccessfulScopeKey(null);
  saveInFlightRef.current = true;
  setSubmittingScopeKey(request.scopeKey);
  try {
    await saveAsync({
      baseVersion: request.baseVersion,
      factory: request.factory,
    });
    if (activeScopeKeyRef.current !== request.scopeKey) {
      return;
    }
    request.onSaved?.();
    setIsConfirming(false);
    setSubmittingScopeKey(null);
    setLastFailedScope(null);
    setLastSuccessfulScopeKey(request.scopeKey);
  } catch (error) {
    setIsConfirming(false);
    setSubmittingScopeKey(null);
    setLastSuccessfulScopeKey(null);
    if (activeScopeKeyRef.current !== request.scopeKey) {
      return;
    }
    setLastFailedScope({
      ...normalizeSaveError(error, {
        fallbackMessage: fallbackErrorMessage,
        mapSaveErrorToFieldErrors,
      }),
      scopeKey: request.scopeKey,
    });
  } finally {
    saveInFlightRef.current = false;
    if (activeScopeKeyRef.current !== request.scopeKey) {
      setSubmittingScopeKey(null);
    }
  }
}

function resolveDetailCardSaveState<
  TFieldErrors extends Record<string, string>,
>({
  hasScopeChanged,
  isConfirming,
  lastFailedScope,
  lastSuccessfulScopeKey,
  scopeKey,
  submittingScopeKey,
}: {
  hasScopeChanged: boolean;
  isConfirming: boolean;
  lastFailedScope: ScopedFactoryDocumentSaveErrorState<TFieldErrors> | null;
  lastSuccessfulScopeKey: string | null;
  scopeKey: string | null;
  submittingScopeKey: string | null;
}): FactoryDocumentSaveState<TFieldErrors> {
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

function useResetExitedSaveScope<
  TFieldErrors extends Record<string, string>,
>({
  scopeKey,
  setIsConfirming,
  setLastFailedScope,
  setLastSuccessfulScopeKey,
}: {
  scopeKey: string | null;
  setIsConfirming: (value: boolean) => void;
  setLastFailedScope: (
    value: ScopedFactoryDocumentSaveErrorState<TFieldErrors> | null,
  ) => void;
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
  isDirty,
  scopeKey,
  setLastSuccessfulScopeKey,
}: {
  isDirty: boolean;
  scopeKey: string | null;
  setLastSuccessfulScopeKey: (
    value: string | null | ((currentScopeKey: string | null) => string | null),
  ) => void;
}) {
  useEffect(() => {
    if (isDirty) {
      setLastSuccessfulScopeKey((currentScopeKey) =>
        currentScopeKey === scopeKey ? null : currentScopeKey,
      );
    }
  }, [isDirty, scopeKey, setLastSuccessfulScopeKey]);
}

function normalizeSaveError<
  TFieldErrors extends Record<string, string>,
>(
  error: unknown,
  {
    fallbackMessage,
    mapSaveErrorToFieldErrors,
  }: {
    fallbackMessage: string;
    mapSaveErrorToFieldErrors?: (
      error: Pick<
        CurrentFactoryDefinitionError,
        "code" | "message" | "targets"
      >,
    ) => TFieldErrors | undefined;
  },
): Pick<
  ScopedFactoryDocumentSaveErrorState<TFieldErrors>,
  "fieldErrors" | "message" | "status"
> {
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
    fieldErrors: mapSaveErrorToFieldErrors?.(error),
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
