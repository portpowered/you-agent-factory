import { useEffect, useState } from "react";

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
      source: "editable-definition",
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
    source: "projection",
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
