import { useCallback, useState } from "react";

import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  createFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  type FactoryGraphAddEntityKind,
  validateFactoryGraphAddEntityDraft,
} from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import { useGraphEditorPlaceAddedNode } from "./graph-editor-placement-context";

export function useFactoryGraphAddEntityController({
  currentFactoryDefinition,
  editableGraph,
  setActiveTool,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  editableGraph: EditableFactoryGraphViewModel;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [addEntityDraft, setAddEntityDraft] =
    useState<FactoryGraphAddEntityDraft | null>(null);
  const [addEntityErrors, setAddEntityErrors] =
    useState<FactoryGraphAddEntityFieldErrors>({});
  const placeAddedNode = useGraphEditorPlaceAddedNode();

  const handleAddEntityAction = useCallback(
    (actionID: string) => {
      setActiveTool("add");
      setAddEntityDraft(
        createFactoryGraphAddEntityDraft(
          actionID as FactoryGraphAddEntityKind,
          currentFactoryDefinition,
        ),
      );
      setAddEntityErrors({});
      setAddMenuOpen(false);
    },
    [currentFactoryDefinition, setActiveTool],
  );

  const handleAddEntitySubmit = useCallback(() => {
    if (addEntityDraft === null) {
      return;
    }

    const validationErrors = validateFactoryGraphAddEntityDraft(
      addEntityDraft,
      currentFactoryDefinition,
    );
    if (Object.keys(validationErrors).length > 0) {
      setAddEntityErrors(validationErrors);
      return;
    }

    const addResult = editableGraph.actions.addNode(addEntityDraft);
    if (!addResult.ok) {
      setAddEntityErrors(addResult.fieldErrors ?? { name: addResult.message });
      return;
    }

    placeAddedNode?.(addEntityDraft);
    setActiveTool(null);
    setAddEntityDraft(null);
    setAddEntityErrors({});
  }, [
    addEntityDraft,
    currentFactoryDefinition,
    editableGraph.actions,
    placeAddedNode,
    setActiveTool,
  ]);

  const reset = useCallback(() => {
    setAddMenuOpen(false);
    setAddEntityDraft(null);
    setAddEntityErrors({});
  }, []);

  return {
    addEntityDraft,
    addEntityErrors,
    addMenuOpen,
    handleAddEntityAction,
    handleAddEntitySubmit,
    reset,
    setAddEntityDraft,
    setAddEntityErrors,
    setAddMenuOpen,
  };
}
