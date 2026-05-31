import type { EditableFactoryGraphSaveMutation } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";

export function useGraphEditorControllers({
  activeTool,
  canInteractWithEditor,
  currentFactoryDefinition,
  draftState,
  editableGraph,
  locale,
  saveEditableDefinition,
  setActiveTool,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  locale?: string | null;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const addEntityController = useFactoryGraphAddEntityController({
    currentFactoryDefinition,
    editableGraph,
    setActiveTool,
  });
  const connectionController = useFactoryGraphConnectionController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    locale,
  });
  const removalController = useFactoryGraphRemovalController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    locale,
    saveEditableDefinition,
  });

  return {
    addEntityController,
    ...connectionController,
    ...removalController,
  };
}
