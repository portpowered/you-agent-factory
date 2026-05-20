import { useCallback, useEffect, useMemo, useState } from "react";

import { buildPendingFactoryDefinition } from "./factory-graph-draft-apply";
import {
  buildFactoryGraphTopologyFromDashboardTopology,
  buildFactoryGraphTopologyFromDefinition,
} from "./factory-graph-draft-graph";
import {
  createEmptyFactoryGraphDraft,
  hasFactoryGraphDraftChanges,
  type EditableFactoryDefinitionDocument,
  type FactoryGraphDraftDerivedState,
  type FactoryGraphDraftSessionState,
  type FactoryGraphDraftValidationError,
  type DashboardTopology,
} from "./factory-graph-draft-types";
import { validateFactoryGraphDraft } from "./factory-graph-draft-validation";

const EMPTY_VALIDATION_ERRORS: FactoryGraphDraftValidationError[] = [];

type FactoryGraphDraftCallbacks = Pick<
  FactoryGraphDraftDerivedState,
  "replaceDraft" | "resetDraft" | "updateDraft"
>;

interface UseFactoryGraphDraftStateOptions {
  editableDefinitionDocument?: EditableFactoryDefinitionDocument;
  projectedTopology?: DashboardTopology;
}

export function useFactoryGraphDraftState(
  options: UseFactoryGraphDraftStateOptions,
): FactoryGraphDraftDerivedState {
  const editableDefinitionDocument = options.editableDefinitionDocument;
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
          editableDefinitionDocument ?? currentState?.latestDocument;
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
    [editableDefinitionDocument],
  );

  const updateDraft = useCallback(
    (
      updater: (
        draft: FactoryGraphDraftSessionState["draft"],
      ) => FactoryGraphDraftSessionState["draft"],
    ) => {
      setSessionState((currentState) => {
        const document =
          editableDefinitionDocument ?? currentState?.latestDocument;
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
    [editableDefinitionDocument],
  );

  const resetDraft = useCallback(() => {
    setSessionState((currentState) => {
      const document = editableDefinitionDocument ?? currentState?.latestDocument;
      if (!document) {
        return currentState;
      }

      return {
        draft: createEmptyFactoryGraphDraft(),
        latestDocument: document,
        sessionStartDocument: document,
      };
    });
  }, [editableDefinitionDocument]);

  useEffect(() => {
    if (!editableDefinitionDocument) {
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
        editableDefinitionDocument,
      ),
    );
  }, [editableDefinitionDocument]);

  const editableDefinitionState =
    useMemo<FactoryGraphDraftDerivedState | null>(
      () =>
        sessionState
          ? createEditableDefinitionFactoryGraphDraftState({
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

  return editableDefinitionState ?? projectionState;
}

function createEditableDefinitionFactoryGraphDraftState({
  callbacks,
  sessionState,
}: {
  callbacks: FactoryGraphDraftCallbacks;
  sessionState: FactoryGraphDraftSessionState;
}): FactoryGraphDraftDerivedState {
  const pendingFactoryDefinition = buildPendingFactoryDefinition(
    sessionState.latestDocument.factoryDefinition,
    sessionState.draft,
  );
  const validationErrors = validateFactoryGraphDraft(
    sessionState.latestDocument.factoryDefinition,
    sessionState.draft,
  );

  return {
    baseDocument: sessionState.sessionStartDocument,
    draft: sessionState.draft,
    graph: buildFactoryGraphTopologyFromDefinition(
      pendingFactoryDefinition ?? sessionState.latestDocument.factoryDefinition,
    ),
    hasChanges: hasFactoryGraphDraftChanges(sessionState.draft),
    latestDocument: sessionState.latestDocument,
    pendingFactoryDefinition,
    replaceDraft: callbacks.replaceDraft,
    resetDraft: callbacks.resetDraft,
    source: "editable-definition",
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
  editableDefinitionDocument: EditableFactoryDefinitionDocument,
): FactoryGraphDraftSessionState {
  if (!currentState || !hasFactoryGraphDraftChanges(currentState.draft)) {
    return {
      draft: createEmptyFactoryGraphDraft(),
      latestDocument: editableDefinitionDocument,
      sessionStartDocument: editableDefinitionDocument,
    };
  }

  if (
    currentState.latestDocument.version.logical ===
      editableDefinitionDocument.version.logical &&
    currentState.latestDocument.version.physical ===
      editableDefinitionDocument.version.physical
  ) {
    return currentState;
  }

  return {
    ...currentState,
    latestDocument: editableDefinitionDocument,
  };
}
