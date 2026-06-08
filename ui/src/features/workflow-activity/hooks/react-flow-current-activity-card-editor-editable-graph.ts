import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useEditableFactoryGraph } from "../../factory-graph-editor/hooks/use-editable-factory-graph";

export function useCurrentActivityEditableGraph({
  editorMode: _editorMode,
  factoryDocumentScopeKey,
  hasPreferenceChanges = false,
  locale,
  snapshot,
}: {
  editorMode: boolean;
  factoryDocumentScopeKey?: string | null;
  hasPreferenceChanges?: boolean;
  locale?: string | null;
  snapshot: DashboardSnapshot;
}) {
  const currentFactoryQuery = useCurrentFactoryDocument(true);
  const editableGraph = useEditableFactoryGraph({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    currentFactoryDocument: currentFactoryQuery.data,
    factoryDocumentScopeKey,
    hasPreferenceChanges,
    locale,
  });

  return {
    currentFactoryQuery,
    editableGraph,
    saveEditableDefinition: editableGraph.saveMutation,
  };
}
