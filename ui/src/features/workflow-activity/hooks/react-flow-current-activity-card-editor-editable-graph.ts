import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useEditableFactoryGraph } from "../../factory-graph-editor/hooks/use-editable-factory-graph";
import type { CurrentActivityFactoryDocumentState } from "./current-activity-factory-document-state";

export function useCurrentActivityEditableGraph({
  editorMode: _editorMode,
  factoryDocumentState,
  factoryDocumentScopeKey,
  hasPreferenceChanges = false,
  locale,
  snapshot,
}: {
  editorMode: boolean;
  factoryDocumentState: CurrentActivityFactoryDocumentState;
  factoryDocumentScopeKey?: string | null;
  hasPreferenceChanges?: boolean;
  locale?: string | null;
  snapshot: DashboardSnapshot;
}) {
  const editableGraph = useEditableFactoryGraph({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    currentFactoryDocument: factoryDocumentState.currentFactoryDocument,
    factoryDocumentScopeKey,
    hasPreferenceChanges,
    locale,
  });

  return {
    editableDefinitionQuery: factoryDocumentState.editableDefinitionQuery,
    editableGraph,
    saveEditableDefinition: editableGraph.saveMutation,
  };
}
