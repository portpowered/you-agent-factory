import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useEditableFactoryGraph } from "../../factory-graph-editor/hooks/use-editable-factory-graph";

export function useCurrentActivityEditableGraph({
  editorMode,
  factoryDocumentScopeKey,
  locale,
  snapshot,
}: {
  editorMode: boolean;
  factoryDocumentScopeKey?: string | null;
  locale?: string | null;
  snapshot: DashboardSnapshot;
}) {
  const currentFactoryQuery = useCurrentFactoryDocument(true);
  const editableGraph = useEditableFactoryGraph({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    currentFactoryDocument: currentFactoryQuery.data,
    factoryDocumentScopeKey,
    locale,
  });

  return {
    currentFactoryQuery,
    editableGraph,
    saveEditableDefinition: editableGraph.saveMutation,
  };
}
