import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  useCurrentFactoryDocument,
  useFactoryDocumentSave,
} from "../../current-factory-definition/public";
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
  const currentFactoryQuery = useCurrentFactoryDocument(editorMode);
  const saveEditableDefinition = useFactoryDocumentSave();
  const editableGraph = useEditableFactoryGraph({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    currentFactoryDocument: currentFactoryQuery.data,
    factoryDocumentScopeKey,
    locale,
    saveFactoryDefinition: (input) =>
      saveEditableDefinition.saveAsync({
        baseVersion: input.baseVersion,
        factory: input.factoryDefinition,
      }),
  });

  return {
    currentFactoryQuery,
    editableGraph,
    saveEditableDefinition,
  };
}
