import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useEditableFactoryGraph } from "../../factory-graph-editor/hooks/use-editable-factory-graph";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { CurrentActivityFactoryDocumentState } from "./current-activity-factory-document-state";
import type { WorkflowActivityBentoCardState } from "./workflow-activity-card-state";

export interface CurrentActivityEditableGraphDocumentDraft {
  baseDocument: EditableFactoryGraphViewModel["draftState"]["baseDocument"];
  canRedoLayout: boolean;
  canUndoLayout: boolean;
  dirtyState: EditableFactoryGraphViewModel["pendingState"]["dirtyState"];
  graph: EditableFactoryGraphViewModel["draftState"]["graph"];
  hasChanges: boolean;
  hasLayoutChanges: boolean;
  hasTopologyChanges: boolean;
  latestDocument: EditableFactoryGraphViewModel["draftState"]["latestDocument"];
  layout: EditableFactoryGraphViewModel["layoutDraftState"]["layout"];
  pendingFactoryDefinition: EditableFactoryGraphViewModel["draftState"]["pendingFactoryDefinition"];
  source: EditableFactoryGraphViewModel["draftState"]["source"];
  topologyDraft: EditableFactoryGraphViewModel["draftState"]["draft"];
  validationErrors: EditableFactoryGraphViewModel["draftState"]["validationErrors"];
}

export function useCurrentActivityEditableGraph({
  editorMode: _editorMode,
  factoryDocumentState,
  factoryDocumentScopeKey,
  hasPreferenceChanges = false,
  locale,
  restoredCardState,
  snapshot,
}: {
  editorMode: boolean;
  factoryDocumentState: CurrentActivityFactoryDocumentState;
  factoryDocumentScopeKey?: string | null;
  hasPreferenceChanges?: boolean;
  locale?: string | null;
  restoredCardState?: WorkflowActivityBentoCardState;
  snapshot: DashboardSnapshot;
}) {
  const editableGraph = useEditableFactoryGraph({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    currentFactoryDocument: factoryDocumentState.currentFactoryDocument,
    factoryDocumentScopeKey,
    hasPreferenceChanges,
    initialDraft: restoredCardState?.topologyDraft,
    initialLayout: restoredCardState?.layout,
    locale,
  });
  const documentDraft: CurrentActivityEditableGraphDocumentDraft = {
    baseDocument: editableGraph.draftState.baseDocument,
    canRedoLayout: editableGraph.pendingState.canRedoLayout,
    canUndoLayout: editableGraph.pendingState.canUndoLayout,
    dirtyState: editableGraph.pendingState.dirtyState,
    graph: editableGraph.draftState.graph,
    hasChanges: editableGraph.pendingState.hasPortableDocumentChanges,
    hasLayoutChanges: editableGraph.pendingState.hasLayoutChanges,
    hasTopologyChanges: editableGraph.pendingState.hasTopologyChanges,
    latestDocument: editableGraph.draftState.latestDocument,
    layout: editableGraph.layoutDraftState.layout,
    pendingFactoryDefinition: editableGraph.draftState.pendingFactoryDefinition,
    source: editableGraph.draftState.source,
    topologyDraft: editableGraph.draftState.draft,
    validationErrors: editableGraph.draftState.validationErrors,
  };

  return {
    documentDraft,
    editableDefinitionQuery: factoryDocumentState.editableDefinitionQuery,
    editableGraph,
    saveEditableDefinition: editableGraph.saveMutation,
  };
}
