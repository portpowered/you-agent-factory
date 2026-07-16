// biome-ignore lint/style/noExcessiveLinesPerFile: removal controller keeps confirm, cancel, and graph/selection delete entry points aligned.
import { useCallback, useEffect, useState } from "react";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type {
  EditableFactoryGraphSaveMutation,
  EditableFactoryGraphViewModel,
} from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { buildFactoryGraphDocRemovalIntent } from "../../factory-graph-editor/lib/factory-graph-doc-editor";
import {
  buildFactoryGraphEdgeRemovalIntent,
  buildFactoryGraphRemovalIntent,
} from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-removals";
import {
  buildFactoryGraphSelectionBatchRemovalPlan,
  hasDeletableFactoryGraphSelection,
  type FactoryGraphSelectionBatchRemovalPlan,
  type FactoryGraphSelectionBatchRemovalSelection,
} from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection-batch-delete";

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
    pendingBatchRemovalPlan,
    pendingRemovalEdgeId,
    pendingRemovalIntent,
    pendingRemovalNodeId,
    setBlockedRemovalReason,
    setPendingBatchRemovalPlan,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  } = usePendingRemovalIntentState(draftState, locale);

  useEffect(() => {
    if (activeTool === "delete") {
      return;
    }

    setBlockedRemovalReason(null);
    setPendingBatchRemovalPlan(null);
    setPendingRemovalEdgeId(null);
    setPendingRemovalNodeId(null);
  }, [
    activeTool,
    setBlockedRemovalReason,
    setPendingBatchRemovalPlan,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);

  const notifyNodeRemovedFromDraft = useCallback(
    (nodeId: string) => {
      onNodeRemovedFromDraft?.(nodeId);
    },
    [onNodeRemovedFromDraft],
  );
  const applyConfirmedBatchRemoval = useCallback(
    (plan: Pick<FactoryGraphSelectionBatchRemovalPlan, "edgeIds" | "nodeIds">) => {
      const result = editableGraph.actions.removeSelection({
        edgeIds: plan.edgeIds,
        nodeIds: plan.nodeIds,
      });
      if (!result.ok) {
        setBlockedRemovalReason(result.message);
        setPendingBatchRemovalPlan(null);
        setPendingRemovalEdgeId(null);
        setPendingRemovalNodeId(null);
        saveEditableDefinition.reset();
        return null;
      }

      setBlockedRemovalReason(null);
      setPendingBatchRemovalPlan(null);
      setPendingRemovalEdgeId(null);
      setPendingRemovalNodeId(null);
      saveEditableDefinition.reset();
      for (const nodeId of plan.nodeIds) {
        notifyNodeRemovedFromDraft(nodeId);
      }
      return {
        edgeIds: plan.edgeIds,
        nodeIds: plan.nodeIds,
      };
    },
    [
      editableGraph.actions,
      notifyNodeRemovedFromDraft,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingBatchRemovalPlan,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    ],
  );
  const applyConfirmedNodeRemoval = useCallback(
    (nodeId: string) => {
      const removed = applyConfirmedBatchRemoval({
        edgeIds: [],
        nodeIds: [nodeId],
      });
      return removed !== null;
    },
    [applyConfirmedBatchRemoval],
  );
  const requestNodeRemoval = useFactoryGraphNodeRemovalRequest({
    applyConfirmedNodeRemoval,
    canInteractWithEditor,
    draftState,
    hiddenNodeClasses,
    locale,
    saveEditableDefinition,
    setBlockedRemovalReason,
    setPendingBatchRemovalPlan,
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
  const handleSelectionBatchDelete = useCallback(
    (selection: FactoryGraphSelectionBatchRemovalSelection) => {
      if (!canInteractWithEditor || !draftState.latestDocument) {
        return { status: "empty" } as const;
      }

      const plan = buildFactoryGraphSelectionBatchRemovalPlan({
        baseFactoryDefinition: draftState.latestDocument,
        draft: draftState.draft,
        hiddenNodeClasses,
        locale,
        selection,
      });
      if (!plan || (plan.nodeIds.length === 0 && plan.edgeIds.length === 0)) {
        return { status: "empty" } as const;
      }
      if (plan.ineligibleReason) {
        setBlockedRemovalReason(plan.ineligibleReason);
        setPendingBatchRemovalPlan(null);
        setPendingRemovalEdgeId(null);
        setPendingRemovalNodeId(null);
        saveEditableDefinition.reset();
        return { status: "blocked" } as const;
      }

      if (plan.confirmation) {
        setBlockedRemovalReason(null);
        setPendingBatchRemovalPlan(plan);
        setPendingRemovalEdgeId(null);
        setPendingRemovalNodeId(null);
        return { status: "pending" } as const;
      }

      const removed = applyConfirmedBatchRemoval(plan);
      if (!removed) {
        return { status: "blocked" } as const;
      }

      return {
        removed,
        status: "applied",
      } as const;
    },
    [
      applyConfirmedBatchRemoval,
      canInteractWithEditor,
      draftState.draft,
      draftState.latestDocument,
      hiddenNodeClasses,
      locale,
      saveEditableDefinition,
      setBlockedRemovalReason,
      setPendingBatchRemovalPlan,
      setPendingRemovalEdgeId,
      setPendingRemovalNodeId,
    ],
  );
  const handleSelectionNodeDelete = useCallback(
    (nodeId: string) => {
      handleSelectionBatchDelete({
        edgeIds: [],
        nodeIds: [nodeId],
      });
    },
    [handleSelectionBatchDelete],
  );
  const canDeleteSelection = useCallback(
    (selection: FactoryGraphSelectionBatchRemovalSelection) =>
      canInteractWithEditor &&
      hasDeletableFactoryGraphSelection({
        baseFactoryDefinition: draftState.latestDocument,
        draft: draftState.draft,
        hiddenNodeClasses,
        locale,
        selection: {
          selectedEdgeIds: new Set(selection.edgeIds),
          selectedNodeIds: new Set(selection.nodeIds),
        },
      }),
    [
      canInteractWithEditor,
      draftState.draft,
      draftState.latestDocument,
      hiddenNodeClasses,
      locale,
    ],
  );

  const handleCancelRemoval = useCallback(() => {
    setBlockedRemovalReason(null);
    setPendingBatchRemovalPlan(null);
    setPendingRemovalEdgeId(null);
    setPendingRemovalNodeId(null);
  }, [
    setBlockedRemovalReason,
    setPendingBatchRemovalPlan,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);

  const handleConfirmRemoval = useCallback(() => {
    if (pendingBatchRemovalPlan) {
      applyConfirmedBatchRemoval(pendingBatchRemovalPlan);
      return;
    }

    if (!pendingRemovalIntent) {
      return;
    }

    if (
      ("key" in pendingRemovalIntent || "targetPath" in pendingRemovalIntent) &&
      pendingRemovalNodeId
    ) {
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
    applyConfirmedBatchRemoval,
    applyConfirmedNodeRemoval,
    editableGraph.actions,
    pendingBatchRemovalPlan,
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
    canDeleteSelection,
    handleCancelRemoval,
    handleConfirmRemoval,
    handleEditorEdgeDelete,
    handleEditorNodeDelete,
    handleSelectionBatchDelete,
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
  setPendingBatchRemovalPlan,
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
  setPendingBatchRemovalPlan: (
    plan: FactoryGraphSelectionBatchRemovalPlan | null,
  ) => void;
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

      const docIntent = buildFactoryGraphDocRemovalIntent({
        baseFactoryDefinition: draftState.latestDocument,
        draft: draftState.draft,
        locale,
        nodeId,
      });
      const intent =
        docIntent ??
        buildFactoryGraphRemovalIntent({
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
        setPendingBatchRemovalPlan(null);
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
      setPendingBatchRemovalPlan,
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
  const [pendingBatchRemovalPlan, setPendingBatchRemovalPlan] =
    useState<FactoryGraphSelectionBatchRemovalPlan | null>(null);
  const [blockedRemovalReason, setBlockedRemovalReason] = useState<
    string | null
  >(null);
  const pendingNodeRemovalIntent =
    draftState.latestDocument && pendingRemovalNodeId
      ? (buildFactoryGraphDocRemovalIntent({
          baseFactoryDefinition: draftState.latestDocument,
          draft: draftState.draft,
          locale,
          nodeId: pendingRemovalNodeId,
        }) ??
        buildFactoryGraphRemovalIntent({
          baseFactoryDefinition: draftState.latestDocument,
          draft: draftState.draft,
          locale,
          nodeId: pendingRemovalNodeId,
        }))
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
    pendingBatchRemovalPlan,
    pendingRemovalEdgeId,
    pendingRemovalIntent:
      pendingBatchRemovalPlan?.confirmation ??
      pendingNodeRemovalIntent ??
      pendingEdgeRemovalIntent,
    pendingRemovalNodeId,
    setBlockedRemovalReason,
    setPendingBatchRemovalPlan,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  };
}
