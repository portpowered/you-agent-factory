import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { buildPendingFactoryDefinition } from "../lib/draft/factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "../lib/draft/factory-graph-draft-graph";
import {
  type CurrentFactoryDocument,
  createEmptyFactoryGraphDraft,
  type FactoryGraphDraft,
  type FactoryGraphDraftDerivedState,
  type FactoryGraphDraftSessionState,
  type FactoryGraphDraftValidationError,
  hasFactoryGraphDraftChanges,
} from "../lib/draft/factory-graph-draft-types";
import { validateFactoryGraphDraft } from "../lib/draft/factory-graph-draft-validation";

const EMPTY_VALIDATION_ERRORS: FactoryGraphDraftValidationError[] = [];

type FactoryGraphDraftCallbacks = Pick<
  FactoryGraphDraftDerivedState,
  "adoptSavedFactoryDocument" | "replaceDraft" | "resetDraft" | "updateDraft"
>;

interface UseFactoryGraphDraftStateOptions {
  currentFactoryDocument?: CurrentFactoryDocument;
  /** Normalized dashboard session id; draft state resets when this changes. */
  factoryDocumentScopeKey?: string | null;
  initialDraft?: FactoryGraphDraft;
  locale?: string | null;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: draft session callbacks and derived projection state share one hook boundary.
export function useFactoryGraphDraftState(
  options: UseFactoryGraphDraftStateOptions,
): FactoryGraphDraftDerivedState {
  const currentFactoryDocument = options.currentFactoryDocument;
  const factoryDocumentScopeKey = options.factoryDocumentScopeKey ?? null;
  const lastFactoryDocumentScopeKeyRef = useRef<string | null>(null);
  const lastSyncedFactoryDocumentRef = useRef<
    CurrentFactoryDocument | undefined
  >(undefined);
  const staleDocumentBlockedForScopeChangeRef = useRef<
    CurrentFactoryDocument | undefined
  >(undefined);
  const initialDraftRef = useRef<FactoryGraphDraft | undefined>(
    options.initialDraft,
  );
  const emptyDraft = useMemo(() => createEmptyFactoryGraphDraft(), []);
  const emptyGraph = useMemo(
    () => ({ edges: [], nodes: [] }) as FactoryGraphDraftDerivedState["graph"],
    [],
  );
  const [sessionState, setSessionState] =
    useState<FactoryGraphDraftSessionState | null>(null);

  const replaceDraft = useCallback(
    (draft: FactoryGraphDraftSessionState["draft"]) => {
      setSessionState((currentState) => {
        const document = currentFactoryDocument ?? currentState?.latestDocument;
        if (!document) {
          return currentState;
        }

        return {
          draft: structuredClone(draft),
          latestDocument: document,
          sessionStartDocument: currentState?.sessionStartDocument ?? document,
        };
      });
    },
    [currentFactoryDocument],
  );

  const updateDraft = useCallback(
    (
      updater: (
        draft: FactoryGraphDraftSessionState["draft"],
      ) => FactoryGraphDraftSessionState["draft"],
    ) => {
      setSessionState((currentState) => {
        const document = currentFactoryDocument ?? currentState?.latestDocument;
        if (!document) {
          return currentState;
        }

        const currentDraft =
          currentState?.draft ?? createEmptyFactoryGraphDraft();

        return {
          draft: structuredClone(updater(currentDraft)),
          latestDocument: document,
          sessionStartDocument: currentState?.sessionStartDocument ?? document,
        };
      });
    },
    [currentFactoryDocument],
  );

  const resetDraft = useCallback(() => {
    setSessionState((currentState) => {
      const document = currentFactoryDocument ?? currentState?.latestDocument;
      if (!document) {
        return currentState;
      }

      return {
        draft: createEmptyFactoryGraphDraft(),
        latestDocument: document,
        sessionStartDocument: document,
      };
    });
  }, [currentFactoryDocument]);

  const adoptSavedFactoryDocument = useCallback(
    (document: CurrentFactoryDocument) => {
      setSessionState({
        draft: createEmptyFactoryGraphDraft(),
        latestDocument: document,
        sessionStartDocument: document,
      });
    },
    [],
  );

  useEffect(() => {
    const previousScopeKey = lastFactoryDocumentScopeKeyRef.current;
    const scopeChanged =
      previousScopeKey !== null && previousScopeKey !== factoryDocumentScopeKey;
    lastFactoryDocumentScopeKeyRef.current = factoryDocumentScopeKey;

    if (scopeChanged) {
      const staleDocument = lastSyncedFactoryDocumentRef.current;
      lastSyncedFactoryDocumentRef.current = undefined;
      setSessionState(null);

      if (
        !currentFactoryDocument ||
        (staleDocument !== undefined &&
          staleDocument === currentFactoryDocument)
      ) {
        staleDocumentBlockedForScopeChangeRef.current = staleDocument;
        return;
      }

      staleDocumentBlockedForScopeChangeRef.current = undefined;
      setSessionState(
        syncFactoryGraphDraftSession(null, currentFactoryDocument),
      );
      lastSyncedFactoryDocumentRef.current = currentFactoryDocument;
      return;
    }

    if (!currentFactoryDocument) {
      staleDocumentBlockedForScopeChangeRef.current = undefined;
      setSessionState(null);
      return;
    }

    if (
      staleDocumentBlockedForScopeChangeRef.current !== undefined &&
      staleDocumentBlockedForScopeChangeRef.current === currentFactoryDocument
    ) {
      return;
    }
    staleDocumentBlockedForScopeChangeRef.current = undefined;

    const restoredDraft = initialDraftRef.current;
    initialDraftRef.current = undefined;
    setSessionState((currentState) => {
      if (currentState) {
        return syncFactoryGraphDraftSession(
          currentState,
          currentFactoryDocument,
        );
      }

      return {
        draft: structuredClone(restoredDraft ?? createEmptyFactoryGraphDraft()),
        latestDocument: currentFactoryDocument,
        sessionStartDocument: currentFactoryDocument,
      };
    });
    lastSyncedFactoryDocumentRef.current = currentFactoryDocument;
  }, [currentFactoryDocument, factoryDocumentScopeKey]);

  const currentFactoryState = useMemo<FactoryGraphDraftDerivedState | null>(
    () =>
      sessionState
        ? createCurrentFactoryGraphDraftState({
            callbacks: {
              adoptSavedFactoryDocument,
              replaceDraft,
              resetDraft,
              updateDraft,
            },
            locale: options.locale,
            sessionState,
          })
        : null,
    [
      adoptSavedFactoryDocument,
      options.locale,
      replaceDraft,
      resetDraft,
      sessionState,
      updateDraft,
    ],
  );

  const unavailableState = useMemo<FactoryGraphDraftDerivedState>(
    () =>
      createUnavailableFactoryGraphDraftState({
        callbacks: {
          adoptSavedFactoryDocument,
          replaceDraft,
          resetDraft,
          updateDraft,
        },
        emptyDraft,
        emptyGraph,
      }),
    [
      adoptSavedFactoryDocument,
      emptyDraft,
      emptyGraph,
      replaceDraft,
      resetDraft,
      updateDraft,
    ],
  );

  return currentFactoryState ?? unavailableState;
}

function createCurrentFactoryGraphDraftState({
  callbacks,
  locale,
  sessionState,
}: {
  callbacks: FactoryGraphDraftCallbacks;
  locale?: string | null;
  sessionState: FactoryGraphDraftSessionState;
}): FactoryGraphDraftDerivedState {
  const pendingFactoryDefinition = buildPendingFactoryDefinition(
    sessionState.latestDocument,
    sessionState.draft,
    locale,
  );
  const validationErrors = validateFactoryGraphDraft(
    sessionState.latestDocument,
    sessionState.draft,
    locale,
  );

  return {
    baseDocument: sessionState.sessionStartDocument,
    draft: sessionState.draft,
    graph: buildFactoryGraphTopologyFromDefinition(
      pendingFactoryDefinition ?? sessionState.latestDocument,
    ),
    hasChanges: hasFactoryGraphDraftChanges(sessionState.draft),
    latestDocument: sessionState.latestDocument,
    pendingFactoryDefinition,
    adoptSavedFactoryDocument: callbacks.adoptSavedFactoryDocument,
    replaceDraft: callbacks.replaceDraft,
    resetDraft: callbacks.resetDraft,
    source: "current-factory",
    updateDraft: callbacks.updateDraft,
    validationErrors,
  };
}

function createUnavailableFactoryGraphDraftState({
  callbacks,
  emptyDraft,
  emptyGraph,
}: {
  callbacks: FactoryGraphDraftCallbacks;
  emptyDraft: FactoryGraphDraftDerivedState["draft"];
  emptyGraph: FactoryGraphDraftDerivedState["graph"];
}): FactoryGraphDraftDerivedState {
  return {
    baseDocument: null,
    draft: emptyDraft,
    graph: emptyGraph,
    hasChanges: false,
    latestDocument: null,
    pendingFactoryDefinition: null,
    adoptSavedFactoryDocument: callbacks.adoptSavedFactoryDocument,
    replaceDraft: callbacks.replaceDraft,
    resetDraft: callbacks.resetDraft,
    source: "projection",
    updateDraft: callbacks.updateDraft,
    validationErrors: EMPTY_VALIDATION_ERRORS,
  };
}

export function syncFactoryGraphDraftSession(
  currentState: FactoryGraphDraftSessionState | null,
  currentFactoryDocument: CurrentFactoryDocument,
): FactoryGraphDraftSessionState {
  if (
    !currentState ||
    !hasFactoryGraphDraftChanges(currentState.draft) ||
    !isSameFactoryDocumentIdentity(
      currentState.latestDocument,
      currentFactoryDocument,
    )
  ) {
    return {
      draft: createEmptyFactoryGraphDraft(),
      latestDocument: currentFactoryDocument,
      sessionStartDocument: currentFactoryDocument,
    };
  }

  if (
    currentState.latestDocument.version.logical ===
      currentFactoryDocument.version.logical &&
    currentState.latestDocument.version.physical ===
      currentFactoryDocument.version.physical
  ) {
    return currentState;
  }

  return {
    ...currentState,
    latestDocument: currentFactoryDocument,
  };
}

function isSameFactoryDocumentIdentity(
  previousDocument: CurrentFactoryDocument,
  nextDocument: CurrentFactoryDocument,
): boolean {
  return previousDocument.name === nextDocument.name;
}
