import { useCallback, useEffect, useMemo, useState } from "react";

import { buildPendingFactoryDefinition } from "../lib/factory-graph-draft-apply";
import {
  buildFactoryGraphTopologyFromDashboardTopology,
  buildFactoryGraphTopologyFromDefinition,
} from "../lib/factory-graph-draft-graph";
import {
  createEmptyFactoryGraphDraft,
  hasFactoryGraphDraftChanges,
  type CurrentFactoryDocument,
  type FactoryGraphDraftDerivedState,
  type FactoryGraphDraftSessionState,
  type FactoryGraphDraftValidationError,
  type DashboardTopology,
} from "../lib/factory-graph-draft-types";
import { validateFactoryGraphDraft } from "../lib/factory-graph-draft-validation";

const EMPTY_VALIDATION_ERRORS: FactoryGraphDraftValidationError[] = [];

type FactoryGraphDraftCallbacks = Pick<
  FactoryGraphDraftDerivedState,
  "replaceDraft" | "resetDraft" | "updateDraft"
>;

interface UseFactoryGraphDraftStateOptions {
  currentFactoryDocument?: CurrentFactoryDocument;
  projectedTopology?: DashboardTopology;
}

export function useFactoryGraphDraftState(
  options: UseFactoryGraphDraftStateOptions,
): FactoryGraphDraftDerivedState {
  const currentFactoryDocument = options.currentFactoryDocument;
  const emptyDraft = useMemo(() => createEmptyFactoryGraphDraft(), []);
  const projectedGraph = useMemo(
    () =>
      buildFactoryGraphTopologyFromDashboardTopology(options.projectedTopology),
    [options.projectedTopology],
  );
  const [sessionState, setSessionState] =
    useState<FactoryGraphDraftSessionState | null>(null);

  const replaceDraft = useCallback(
    (draft: FactoryGraphDraftSessionState["draft"]) => {
      setSessionState((currentState) => {
        const document =
          currentFactoryDocument ?? currentState?.latestDocument;
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
        const document =
          currentFactoryDocument ?? currentState?.latestDocument;
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
      syncFactoryGraphDraftSession(
        currentState,
        currentFactoryDocument,
      ),
    );
  }, [currentFactoryDocument]);

  const currentFactoryState =
    useMemo<FactoryGraphDraftDerivedState | null>(
      () =>
        sessionState
          ? createCurrentFactoryGraphDraftState({
              callbacks: {
                replaceDraft,
                resetDraft,
                updateDraft,
              },
              sessionState,
            })
          : null,
      [replaceDraft, resetDraft, sessionState, updateDraft],
    );

  const projectionState = useMemo<FactoryGraphDraftDerivedState>(
    () =>
      createProjectionFactoryGraphDraftState({
        callbacks: {
          replaceDraft,
          resetDraft,
          updateDraft,
        },
        emptyDraft,
        projectedGraph,
      }),
    [emptyDraft, projectedGraph, replaceDraft, resetDraft, updateDraft],
  );

  return currentFactoryState ?? projectionState;
}

function createCurrentFactoryGraphDraftState({
  callbacks,
  sessionState,
}: {
  callbacks: FactoryGraphDraftCallbacks;
  sessionState: FactoryGraphDraftSessionState;
}): FactoryGraphDraftDerivedState {
  const pendingFactoryDefinition = buildPendingFactoryDefinition(
    sessionState.latestDocument,
    sessionState.draft,
  );
  const validationErrors = validateFactoryGraphDraft(
    sessionState.latestDocument,
    sessionState.draft,
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

function createProjectionFactoryGraphDraftState({
  callbacks,
  emptyDraft,
  projectedGraph,
}: {
  callbacks: FactoryGraphDraftCallbacks;
  emptyDraft: FactoryGraphDraftDerivedState["draft"];
  projectedGraph: FactoryGraphDraftDerivedState["graph"];
}): FactoryGraphDraftDerivedState {
  return {
    baseDocument: null,
    draft: emptyDraft,
    graph: projectedGraph,
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
