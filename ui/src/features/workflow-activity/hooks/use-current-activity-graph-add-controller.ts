import { useCallback, useState } from "react";

import { buildDocTargetPathFromFileName } from "../../current-factory-definition/lib/doc-editable-values";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  createFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  type FactoryGraphAddEntityKind,
  validateFactoryGraphAddEntityDraft,
} from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import type { WorkflowActivityBentoCardState } from "./workflow-activity-card-state";

export interface FactoryGraphAddNodeLayoutPlacement {
  nodeId: string;
  position: { x: number; y: number };
}

export function useFactoryGraphAddEntityController({
  currentFactoryDefinition,
  editableGraph,
  onDocAdded,
  restoredCardState,
  setActiveTool,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  editableGraph: EditableFactoryGraphViewModel;
  onDocAdded?: (targetPath: string) => void;
  restoredCardState?: Pick<
    WorkflowActivityBentoCardState,
    "addEntityDraft" | "addEntityErrors" | "addMenuOpen"
  >;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const [addMenuOpen, setAddMenuOpen] = useState(
    () => restoredCardState?.addMenuOpen ?? false,
  );
  const [addEntityDraft, setAddEntityDraft] =
    useState<FactoryGraphAddEntityDraft | null>(
      () => restoredCardState?.addEntityDraft ?? null,
    );
  const [addEntityErrors, setAddEntityErrors] =
    useState<FactoryGraphAddEntityFieldErrors>(
      () => restoredCardState?.addEntityErrors ?? {},
    );

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

  const handleAddEntitySubmit = useCallback(
    (placement?: FactoryGraphAddNodeLayoutPlacement | null) => {
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
        setAddEntityErrors(
          addResult.fieldErrors ?? { name: addResult.message },
        );
        return;
      }

      if (placement) {
        editableGraph.actions.moveLayoutNode(
          placement.nodeId,
          placement.position,
        );
      }
      if (addEntityDraft.kind === "doc") {
        onDocAdded?.(
          buildDocTargetPathFromFileName(addEntityDraft.fileName.trim()),
        );
      }
      setActiveTool(null);
      setAddEntityDraft(null);
      setAddEntityErrors({});
    },
    [
      addEntityDraft,
      currentFactoryDefinition,
      editableGraph.actions,
      onDocAdded,
      setActiveTool,
    ],
  );

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
