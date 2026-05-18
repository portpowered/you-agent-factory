import { useCallback, useEffect, useState } from "react";

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
  type DashboardTopology,
} from "./factory-graph-draft-types";
import { validateFactoryGraphDraft } from "./factory-graph-draft-validation";

export function useFactoryGraphDraftState(options: {
  editableDefinitionDocument?: EditableFactoryDefinitionDocument;
  projectedTopology?: DashboardTopology;
}): FactoryGraphDraftDerivedState {
  const editableDefinitionDocument = options.editableDefinitionDocument;
  const [sessionState, setSessionState] =
    useState<FactoryGraphDraftSessionState | null>(null);

  const replaceDraft = useCallback((draft: FactoryGraphDraftSessionState["draft"]) => {
    setSessionState((currentState) => {
      const document = editableDefinitionDocument ?? currentState?.latestDocument;
      if (!document) {
        return currentState;
      }

      return {
        draft: structuredClone(draft),
        latestDocument: document,
        sessionStartDocument: currentState?.sessionStartDocument ?? document,
      };
    });
  }, [editableDefinitionDocument]);

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

  if (sessionState) {
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
        pendingFactoryDefinition ??
          sessionState.latestDocument.factoryDefinition,
      ),
      hasChanges: hasFactoryGraphDraftChanges(sessionState.draft),
      latestDocument: sessionState.latestDocument,
      pendingFactoryDefinition,
      replaceDraft,
      resetDraft,
      source: "editable-definition",
      updateDraft,
      validationErrors,
    };
  }

  return {
    baseDocument: null,
    draft: createEmptyFactoryGraphDraft(),
    graph: buildFactoryGraphTopologyFromDashboardTopology(
      options.projectedTopology,
    ),
    hasChanges: false,
    latestDocument: null,
    pendingFactoryDefinition: null,
    replaceDraft,
    resetDraft,
    source: "projection",
    updateDraft,
    validationErrors: [],
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
