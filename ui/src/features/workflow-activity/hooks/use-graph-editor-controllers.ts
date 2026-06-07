import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type {
  EditableFactoryGraphSaveMutation,
  EditableFactoryGraphViewModel,
} from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";

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
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const addEntityController = useFactoryGraphAddEntityController({
    currentFactoryDefinition,
    editableGraph,
    onDocAdded,
    setActiveTool,
  });
  const connectionController = useFactoryGraphConnectionController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    hiddenNodeClasses,
    locale,
  });
  const removalController = useFactoryGraphRemovalController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    hiddenNodeClasses,
    locale,
    onNodeRemovedFromDraft,
    saveEditableDefinition,
  });

  return {
    addEntityController,
    ...connectionController,
    ...removalController,
  };
}
