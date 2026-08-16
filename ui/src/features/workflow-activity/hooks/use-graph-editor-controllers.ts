import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type {
  EditableFactoryGraphSaveMutation,
  EditableFactoryGraphViewModel,
} from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";
import { composeGraphEditorControllers } from "./state/graph-editor-controller-composition";
import { useFactoryGraphAddEntityController } from "./use-current-activity-graph-add-controller";
import type { WorkflowActivityBentoCardState } from "./workflow-activity-card-state";

export function useGraphEditorControllers({
  activeTool,
  canInteractWithEditor,
  currentFactoryDefinition,
  draftState,
  editableGraph,
  hiddenNodeClasses,
  locale,
  onDocAdded,
  onNodeRemovedFromDraft,
  restoredCardState,
  saveEditableDefinition,
  setActiveTool,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  onDocAdded?: (targetPath: string) => void;
  onNodeRemovedFromDraft?: (nodeId: string) => void;
  restoredCardState?: WorkflowActivityBentoCardState;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const addEntityController = useFactoryGraphAddEntityController({
    currentFactoryDefinition,
    editableGraph,
    onDocAdded,
    restoredCardState,
    setActiveTool,
  });
  const connectionController = useFactoryGraphConnectionController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    hiddenNodeClasses,
    locale,
    restoredCardState,
  });
  const removalController = useFactoryGraphRemovalController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    hiddenNodeClasses,
    locale,
    onNodeRemovedFromDraft,
    restoredCardState,
    saveEditableDefinition,
  });

  return composeGraphEditorControllers(
    addEntityController,
    connectionController,
    removalController,
  );
}
