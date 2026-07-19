import { useCallback, useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphEditorVisibilityPreset } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { useFactoryValidation } from "../../factory-graph-editor/hooks/validation/use-factory-validation";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  createDefaultFactoryLayout,
  factoryLayoutFromDefinition,
} from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { buildVisibleGraphEdgesWithDraft } from "../lib/react-flow-current-activity-card-draft-edges";
import type { CurrentActivityFactoryDocumentState } from "./current-activity-factory-document-state";
import type { CurrentActivityGraphValidationState } from "./current-activity-graph-state-value";
import type { CurrentActivityEditableGraphDocumentDraft } from "./react-flow-current-activity-card-editor-editable-graph";
import { useCurrentActivityFactoryGraphViewState } from "./use-current-activity-factory-graph-view-state";
import {
  type CurrentActivityGraphFlowProjection,
  type CurrentActivityGraphProjectionState,
  useCurrentActivityGraphFlowProjection,
} from "./use-current-activity-graph-flow-projection";

type CurrentActivityGraphDefinitionStatus = "error" | "pending" | "success";

export type CurrentActivityGraphLayoutState = {
  canMoveLayout: boolean;
  canRedo?: boolean;
  canUndo?: boolean;
  canonicalViewport: CurrentActivityGraphFlowProjection["canonicalLayoutViewport"];
  currentLayout: CurrentActivityGraphFlowProjection["renderedLayout"];
};

export function useCurrentActivityGraphRenderState({
  documentDraft,
  editorMode,
  factoryDocumentState,
  hiddenNodeClasses,
  structuralValidation,
  snapshot,
  definitionStatus,
  isSaving,
  visibilityPreset,
}: {
  documentDraft: CurrentActivityEditableGraphDocumentDraft;
  editorMode: boolean;
  factoryDocumentState: CurrentActivityFactoryDocumentState;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  structuralValidation: ReturnType<typeof useFactoryValidation>;
  snapshot: DashboardSnapshot;
  definitionStatus: CurrentActivityGraphDefinitionStatus;
  isSaving: boolean;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
}) {
  const viewState = useCurrentActivityFactoryGraphViewState({
    baseFactoryDocument: documentDraft.baseDocument,
    editableFactoryDocument: factoryDocumentState.currentFactoryDocument,
    editableFactoryDocumentStatus: definitionStatus,
    editorMode,
    latestFactoryDocument: documentDraft.latestDocument,
    pendingFactoryDefinition: documentDraft.pendingFactoryDefinition,
  });
  const resolveGraphEdges = useCallback(
    (positionedGraphLayout: GraphLayout) =>
      buildVisibleGraphEdgesWithDraft({
        draft: documentDraft.topologyDraft,
        graphLayout: positionedGraphLayout,
      }),
    [documentDraft.topologyDraft],
  );
  const savedFactoryLayout = useMemo(
    () => factoryLayoutFromDefinition(viewState.displayFactoryDefinition),
    [viewState.displayFactoryDefinition],
  );
  const computedLayout = useMemo(
    () =>
      documentDraft.source === "current-factory"
        ? (documentDraft.layout ?? createDefaultFactoryLayout())
        : savedFactoryLayout,
    [documentDraft.layout, documentDraft.source, savedFactoryLayout],
  );
  const projectionState = useMemo<CurrentActivityGraphProjectionState>(
    () => ({
      computedLayout,
      displayFactoryDefinition: viewState.displayFactoryDefinition,
      resolveGraphEdges,
    }),
    [computedLayout, resolveGraphEdges, viewState.displayFactoryDefinition],
  );
  const flowState = useCurrentActivityGraphFlowProjection({
    hiddenNodeClasses,
    projectionState,
    snapshot,
    visibilityPreset,
  });
  const sessionState = useMemo(
    () => ({
      currentFactoryDefinition: viewState.currentFactoryDefinition ?? null,
      definitionStatus,
      hasPendingGraphChanges: documentDraft.hasChanges,
      isSaving,
      projectedFactory:
        viewState.persistedFactoryDefinition ?? snapshot.factory,
    }),
    [
      definitionStatus,
      documentDraft.hasChanges,
      isSaving,
      snapshot.factory,
      viewState.currentFactoryDefinition,
      viewState.persistedFactoryDefinition,
    ],
  );
  const layoutState = useMemo<CurrentActivityGraphLayoutState>(
    () => ({
      canMoveLayout: documentDraft.source === "current-factory",
      canRedo: documentDraft.canRedoLayout,
      canUndo: documentDraft.canUndoLayout,
      canonicalViewport: flowState.canonicalLayoutViewport,
      currentLayout: flowState.renderedLayout,
    }),
    [
      documentDraft.canRedoLayout,
      documentDraft.canUndoLayout,
      documentDraft.source,
      flowState.canonicalLayoutViewport,
      flowState.renderedLayout,
    ],
  );
  const validationState = useMemo<CurrentActivityGraphValidationState>(
    () => ({
      draftErrors: documentDraft.validationErrors,
      factoryDefinition:
        viewState.pendingFactoryDefinition ??
        viewState.currentFactoryDefinition ??
        flowState.displayFactoryDefinition ??
        null,
      projection: structuralValidation.projection,
      targets: structuralValidation.targets,
    }),
    [
      documentDraft.validationErrors,
      flowState.displayFactoryDefinition,
      structuralValidation.projection,
      structuralValidation.targets,
      viewState.currentFactoryDefinition,
      viewState.pendingFactoryDefinition,
    ],
  );

  return {
    flowState,
    layoutState,
    sessionState,
    validationState,
    viewState,
  };
}
