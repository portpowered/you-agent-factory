import { useCallback, useState } from "react";

import type { useSaveCurrentFactory } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import {
  buildFactoryGraphEdgeRemovalIntent,
  buildFactoryGraphRemovalIntent,
} from "../../factory-graph-editor/lib/factory-graph-editor-removals";

export function useFactoryGraphRemovalController({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  locale,
  saveEditableDefinition,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  locale?: string | null;
  saveEditableDefinition: ReturnType<typeof useSaveCurrentFactory>;
}) {
  const {
    blockedRemovalReason,
    pendingRemovalEdgeId,
    pendingRemovalIntent,
    pendingRemovalNodeId,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  } = usePendingRemovalIntentState(draftState, locale);
  const { handleEditorEdgeDelete, handleEditorNodeDelete } =
    useFactoryGraphDeleteTargetHandlers({
      activeTool,
      canInteractWithEditor,
      draftState,
      editableGraph,
      locale,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    });

  const handleConfirmRemoval = useCallback(() => {
    if (!pendingRemovalIntent) {
      return;
    }

    if ("key" in pendingRemovalIntent && pendingRemovalNodeId) {
      const result = editableGraph.actions.removeNode(pendingRemovalNodeId);
      if (!result.ok) {
        setBlockedRemovalReason(result.message);
        return;
      }
    } else if (pendingRemovalEdgeId) {
      const result = editableGraph.actions.disconnectEdge(pendingRemovalEdgeId);
      if (!result.ok) {
        setBlockedRemovalReason(result.message);
        return;
      }
    } else {
      return;
    }
    setBlockedRemovalReason(null);
    setPendingRemovalEdgeId(null);
    setPendingRemovalNodeId(null);
  }, [
    editableGraph.actions,
    pendingRemovalIntent,
    pendingRemovalEdgeId,
    pendingRemovalNodeId,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);

  return {
    blockedRemovalReason,
    handleConfirmRemoval,
    handleEditorEdgeDelete,
    handleEditorNodeDelete,
    pendingRemovalIntent,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  };
}

function useFactoryGraphDeleteTargetHandlers({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  locale,
  saveEditableDefinition,
  setBlockedRemovalReason,
  setPendingRemovalEdgeId,
  setPendingRemovalNodeId,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  locale?: string | null;
  saveEditableDefinition: ReturnType<typeof useSaveCurrentFactory>;
  setBlockedRemovalReason: (reason: string | null) => void;
  setPendingRemovalEdgeId: (edgeId: string | null) => void;
  setPendingRemovalNodeId: (nodeId: string | null) => void;
}) {
  const handleEditorNodeDelete = useCallback(
    (nodeId: string) => {
      if (
        !canInteractWithEditor ||
        activeTool !== "delete" ||
        !draftState.latestDocument
      ) {
        return;
      }

      const intent = buildFactoryGraphRemovalIntent({
        baseFactoryDefinition: draftState.latestDocument,
        draft: draftState.draft,
        locale,
        nodeId,
      });
      if (!intent) {
        return;
      }
      if (intent.ineligibleReason) {
        editableGraph.actions.removeNode(nodeId);
        setBlockedRemovalReason(intent.ineligibleReason);
        saveEditableDefinition.reset();
        return;
      }

      const result = editableGraph.actions.removeNode(nodeId);
      if (!result.ok) {
        setBlockedRemovalReason(result.message);
        saveEditableDefinition.reset();
        return;
      }
      setBlockedRemovalReason(null);
      setPendingRemovalEdgeId(null);
      setPendingRemovalNodeId(null);
      saveEditableDefinition.reset();
    },
    [
      activeTool,
      canInteractWithEditor,
      draftState.draft,
      draftState.latestDocument,
      editableGraph.actions,
      locale,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    ],
  );
  const handleEditorEdgeDelete = useCallback(
    (edgeId: string) => {
      if (
        !canInteractWithEditor ||
        activeTool !== "delete" ||
        !draftState.latestDocument
      ) {
        return;
      }

      const intent = buildFactoryGraphEdgeRemovalIntent({
        baseFactoryDefinition: draftState.latestDocument,
        draft: draftState.draft,
        edgeId,
        locale,
      });
      if (!intent) {
        return;
      }
      if (intent.ineligibleReason) {
        editableGraph.actions.disconnectEdge(edgeId);
        setBlockedRemovalReason(intent.ineligibleReason);
        saveEditableDefinition.reset();
        return;
      }

      const result = editableGraph.actions.disconnectEdge(edgeId);
      if (!result.ok) {
        setBlockedRemovalReason(result.message);
        saveEditableDefinition.reset();
        return;
      }
      setBlockedRemovalReason(null);
      setPendingRemovalNodeId(null);
      setPendingRemovalEdgeId(null);
      saveEditableDefinition.reset();
    },
    [
      activeTool,
      canInteractWithEditor,
      draftState.draft,
      draftState.latestDocument,
      editableGraph.actions,
      locale,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    ],
  );

  return {
    handleEditorEdgeDelete,
    handleEditorNodeDelete,
  };
}

function usePendingRemovalIntentState(
  draftState: EditableFactoryGraphViewModel["draftState"],
  locale?: string | null,
) {
  const [pendingRemovalNodeId, setPendingRemovalNodeId] = useState<
    string | null
  >(null);
  const [pendingRemovalEdgeId, setPendingRemovalEdgeId] = useState<
    string | null
  >(null);
  const [blockedRemovalReason, setBlockedRemovalReason] = useState<
    string | null
  >(null);
  const pendingNodeRemovalIntent =
    draftState.latestDocument && pendingRemovalNodeId
      ? buildFactoryGraphRemovalIntent({
          baseFactoryDefinition: draftState.latestDocument,
          draft: draftState.draft,
          locale,
          nodeId: pendingRemovalNodeId,
        })
      : null;
  const pendingEdgeRemovalIntent =
    draftState.latestDocument && pendingRemovalEdgeId
      ? buildFactoryGraphEdgeRemovalIntent({
          baseFactoryDefinition: draftState.latestDocument,
          draft: draftState.draft,
          edgeId: pendingRemovalEdgeId,
          locale,
        })
      : null;

  return {
    blockedRemovalReason,
    pendingRemovalEdgeId,
    pendingRemovalIntent: pendingNodeRemovalIntent ?? pendingEdgeRemovalIntent,
    pendingRemovalNodeId,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  };
}
