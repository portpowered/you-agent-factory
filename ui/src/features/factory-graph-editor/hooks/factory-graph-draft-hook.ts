import { useCallback, useEffect, useMemo, useState } from "react";

import { buildPendingFactoryDefinition } from "../lib/factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "../lib/factory-graph-draft-graph";
import {
  type CurrentFactoryDocument,
  createEmptyFactoryGraphDraft,
  type FactoryGraphDraftDerivedState,
  type FactoryGraphDraftSessionState,
  type FactoryGraphDraftValidationError,
  hasFactoryGraphDraftChanges,
} from "../lib/factory-graph-draft-types";
import { validateFactoryGraphDraft } from "../lib/factory-graph-draft-validation";

const EMPTY_VALIDATION_ERRORS: FactoryGraphDraftValidationError[] = [];

type FactoryGraphDraftCallbacks = Pick<
  FactoryGraphDraftDerivedState,
  "replaceDraft" | "resetDraft" | "updateDraft"
>;

interface UseFactoryGraphDraftStateOptions {
  currentFactoryDocument?: CurrentFactoryDocument;
  locale?: string | null;
}

export function useFactoryGraphDraftState(
  options: UseFactoryGraphDraftStateOptions,
): FactoryGraphDraftDerivedState {
  const currentFactoryDocument = options.currentFactoryDocument;
  const emptyDraft = useMemo(() => createEmptyFactoryGraphDraft(), []);
  const emptyGraph = useMemo(
    () => ({ edges: [], nodes: [] } as FactoryGraphDraftDerivedState["graph"]),
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

  useEffect(() => {
    if (!currentFactoryDocument) {
      setSessionState((currentState) =>
        currentState && hasFactoryGraphDraftChanges(currentState.draft)
          ? currentState
          : null,
      );
      return;
    }

    setSessionState((currentState) =>
      syncFactoryGraphDraftSession(currentState, currentFactoryDocument),
    );
  }, [currentFactoryDocument]);

  const currentFactoryState = useMemo<FactoryGraphDraftDerivedState | null>(
    () =>
      sessionState
        ? createCurrentFactoryGraphDraftState({
            callbacks: {
              replaceDraft,
              resetDraft,
              updateDraft,
            },
            locale: options.locale,
            sessionState,
          })
        : null,
    [options.locale, replaceDraft, resetDraft, sessionState, updateDraft],
  );

  const unavailableState = useMemo<FactoryGraphDraftDerivedState>(
    () =>
      createUnavailableFactoryGraphDraftState({
        callbacks: {
          replaceDraft,
          resetDraft,
          updateDraft,
        },
        emptyDraft,
        emptyGraph,
      }),
    [emptyDraft, emptyGraph, replaceDraft, resetDraft, updateDraft],
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
  if (!currentState || !hasFactoryGraphDraftChanges(currentState.draft)) {
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
