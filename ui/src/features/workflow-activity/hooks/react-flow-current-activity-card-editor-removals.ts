import { useCallback, useEffect, useState } from "react";

import type { EditableFactoryGraphSaveMutation } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import {
  buildFactoryGraphEdgeRemovalIntent,
  buildFactoryGraphRemovalIntent,
} from "../../factory-graph-editor/lib/factory-graph-editor-removals";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: removal controller keeps confirm, cancel, and graph/selection delete entry points aligned.
export function useFactoryGraphRemovalController({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  hiddenNodeClasses,
  locale,
  onNodeRemovedFromDraft,
  saveEditableDefinition,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  onNodeRemovedFromDraft?: (nodeId: string) => void;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
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

  useEffect(() => {
    if (activeTool === "delete") {
      return;
    }

    setBlockedRemovalReason(null);
    setPendingRemovalEdgeId(null);
    setPendingRemovalNodeId(null);
  }, [
    activeTool,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);

  const notifyNodeRemovedFromDraft = useCallback(
    (nodeId: string) => {
      onNodeRemovedFromDraft?.(nodeId);
    },
    [onNodeRemovedFromDraft],
  );
  const applyConfirmedNodeRemoval = useCallback(
    (nodeId: string) => {
      const result = editableGraph.actions.removeNode(nodeId);
      if (!result.ok) {
        setBlockedRemovalReason(result.message);
        setPendingRemovalEdgeId(null);
        setPendingRemovalNodeId(null);
        saveEditableDefinition.reset();
        return false;
      }

      setBlockedRemovalReason(null);
      setPendingRemovalEdgeId(null);
      setPendingRemovalNodeId(null);
      saveEditableDefinition.reset();
      notifyNodeRemovedFromDraft(nodeId);
      return true;
    },
    [
      editableGraph.actions,
      notifyNodeRemovedFromDraft,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    ],
  );
  const requestNodeRemoval = useFactoryGraphNodeRemovalRequest({
    applyConfirmedNodeRemoval,
    canInteractWithEditor,
    draftState,
    hiddenNodeClasses,
    locale,
    saveEditableDefinition,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  });
  const { handleEditorEdgeDelete, handleEditorNodeDelete } =
    useFactoryGraphDeleteTargetHandlers({
      activeTool,
      canInteractWithEditor,
      draftState,
      editableGraph,
      locale,
      requestNodeRemoval,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    });
  const handleSelectionNodeDelete = useCallback(
    (nodeId: string) => {
      requestNodeRemoval(nodeId);
    },
    [requestNodeRemoval],
  );

  const handleCancelRemoval = useCallback(() => {
    setBlockedRemovalReason(null);
    setPendingRemovalEdgeId(null);
    setPendingRemovalNodeId(null);
  }, [
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);

  const handleConfirmRemoval = useCallback(() => {
    if (!pendingRemovalIntent) {
      return;
    }

    if ("key" in pendingRemovalIntent && pendingRemovalNodeId) {
      applyConfirmedNodeRemoval(pendingRemovalNodeId);
      return;
    }

    if (pendingRemovalEdgeId) {
      const result = editableGraph.actions.disconnectEdge(pendingRemovalEdgeId);
      if (!result.ok) {
        setBlockedRemovalReason(result.message);
        setPendingRemovalEdgeId(null);
        setPendingRemovalNodeId(null);
        saveEditableDefinition.reset();
        return;
      }
      setBlockedRemovalReason(null);
      setPendingRemovalEdgeId(null);
      setPendingRemovalNodeId(null);
      saveEditableDefinition.reset();
      return;
    }
  }, [
    applyConfirmedNodeRemoval,
    editableGraph.actions,
    pendingRemovalEdgeId,
    pendingRemovalIntent,
    pendingRemovalNodeId,
    saveEditableDefinition,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);

  return {
    blockedRemovalReason,
    handleCancelRemoval,
    handleConfirmRemoval,
    handleEditorEdgeDelete,
    handleEditorNodeDelete,
    handleSelectionNodeDelete,
    pendingRemovalIntent,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  };
}

function useFactoryGraphNodeRemovalRequest({
  applyConfirmedNodeRemoval,
  canInteractWithEditor,
  draftState,
  hiddenNodeClasses,
  locale,
  saveEditableDefinition,
  setBlockedRemovalReason,
  setPendingRemovalEdgeId,
  setPendingRemovalNodeId,
}: {
  applyConfirmedNodeRemoval: (nodeId: string) => boolean;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
  setBlockedRemovalReason: (reason: string | null) => void;
  setPendingRemovalEdgeId: (edgeId: string | null) => void;
  setPendingRemovalNodeId: (nodeId: string | null) => void;
}) {
  return useCallback(
    (nodeId: string) => {
      if (!canInteractWithEditor || !draftState.latestDocument) {
        return;
      }

      const node = draftState.graph.nodes.find((entry) => entry.id === nodeId);
      if (node && hiddenNodeClasses.has(node.kind)) {
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
        setBlockedRemovalReason(intent.ineligibleReason);
        saveEditableDefinition.reset();
        return;
      }

      if (intent.requiresConfirmation) {
        setBlockedRemovalReason(null);
        setPendingRemovalEdgeId(null);
        setPendingRemovalNodeId(nodeId);
        return;
      }

      applyConfirmedNodeRemoval(nodeId);
    },
    [
      applyConfirmedNodeRemoval,
      canInteractWithEditor,
      draftState.draft,
      draftState.graph.nodes,
      draftState.latestDocument,
      hiddenNodeClasses,
      locale,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    ],
  );
}

function useFactoryGraphDeleteTargetHandlers({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  locale,
  requestNodeRemoval,
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
  requestNodeRemoval: (nodeId: string) => void;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
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

      requestNodeRemoval(nodeId);
    },
    [
      activeTool,
      canInteractWithEditor,
      draftState.latestDocument,
      requestNodeRemoval,
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
        setBlockedRemovalReason(intent.ineligibleReason);
        saveEditableDefinition.reset();
        return;
      }

      if (intent.requiresConfirmation) {
        setBlockedRemovalReason(null);
        setPendingRemovalNodeId(null);
        setPendingRemovalEdgeId(edgeId);
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
