import { useCallback, useMemo, useState } from "react";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
} from "../lib/draft/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/editor/factory-graph-editor-additions";
import { createFactoryGraphWorkstationResolver } from "../lib/editor/factory-graph-editor-connections";
import { resolveFactoryGraphEditorDirtyState } from "../lib/editor-runtime/factory-graph-editor-dirty-state";
import type { FactoryGraphNodeFieldUpdate } from "../lib/editor-runtime/factory-graph-field-operations";
import { updateFactoryGraphNodeField } from "../lib/editor-runtime/factory-graph-field-operations";
import { resolveProjectedLayoutPositions } from "../lib/layout/factory-graph-layout-operations";
import {
  addFactoryGraphNode,
  buildFactoryGraphState,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
  type FactoryGraphOperationResult,
  projectFactoryGraphToReactFlow,
  removeFactoryGraphNode,
  removeFactoryGraphSelection,
} from "../lib/operations/factory-graph-operations";
import { useFactoryGraphDraftState } from "./factory-graph-draft-hook";
import { useFactoryGraphLayoutDraftState } from "./layout/factory-graph-layout-draft-hook";
import { useEditableFactoryGraphSaveController } from "./use-editable-factory-graph-save-controller";
import type {
  EditableFactoryGraphViewModel,
  UseEditableFactoryGraphOptions,
} from "./use-editable-factory-graph-types";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: composes draft, layout, projection, and save controllers for the editor.
export function useEditableFactoryGraph(
  options: UseEditableFactoryGraphOptions & {
    hasPreferenceChanges?: boolean;
  },
): EditableFactoryGraphViewModel {
  const [blockedOperation, setBlockedOperation] =
    useState<FactoryGraphOperationResult<never> | null>(null);
  const draftState = useFactoryGraphDraftState(options);
  const layoutDraftState = useFactoryGraphLayoutDraftState(options);
  const baseFactoryDefinition =
    draftState.latestDocument ?? draftState.baseDocument ?? null;
  const graphState = useEditableFactoryGraphState({
    baseFactoryDefinition,
    draftState,
    locale: options.locale,
  });
  const projection = useMemo(() => {
    const factoryDefinition =
      draftState.pendingFactoryDefinition ?? draftState.baseDocument ?? null;
    const layoutPositionsByNodeId = resolveProjectedLayoutPositions({
      autoLayoutPositionsByNodeId: new Map(),
      canonicalLayout: layoutDraftState.layout,
      nodeIds: draftState.graph.nodes.map((node) => node.id),
    });

    return projectFactoryGraphToReactFlow({
      factoryDefinition,
      filterEdgesToRenderedHandles: true,
      layoutPositionsByNodeId,
      locale: options.locale ?? undefined,
      topology: draftState.graph,
      workstationResolver: factoryDefinition
        ? createFactoryGraphWorkstationResolver(
            factoryDefinition.workstations,
            factoryDefinition.workers,
          )
        : undefined,
    });
  }, [
    draftState.baseDocument,
    draftState.graph,
    draftState.pendingFactoryDefinition,
    layoutDraftState.layout,
    options.locale,
  ]);
  const saveController = useEditableFactoryGraphSaveController({
    activeWorkCount: options.activeWorkCount ?? 0,
    draftState,
    factoryDocumentScopeKey: options.factoryDocumentScopeKey ?? null,
    layoutDraftState,
    locale: options.locale,
    setBlockedOperation,
  });
  const mutationActions = useEditableFactoryGraphMutationActions({
    baseFactoryDefinition,
    draftState,
    layoutDraftState,
    locale: options.locale,
    setBlockedOperation,
  });
  const discard = useCallback(() => {
    draftState.resetDraft();
    layoutDraftState.resetLayout({ recordHistory: false });
    setBlockedOperation(null);
    saveController.resetSaveState();
  }, [draftState, layoutDraftState, saveController]);
  const dirtyState = useMemo(
    () =>
      resolveFactoryGraphEditorDirtyState({
        hasLayoutChanges: layoutDraftState.layoutDirty,
        hasPreferenceChanges: options.hasPreferenceChanges ?? false,
        hasTopologyChanges: draftState.hasChanges,
      }),
    [
      draftState.hasChanges,
      layoutDraftState.layoutDirty,
      options.hasPreferenceChanges,
    ],
  );
  const hasPortableDocumentChanges =
    dirtyState.layoutDirty || dirtyState.topologyDirty;

  return {
    actions: {
      addNode: mutationActions.addNode,
      connectNodes: mutationActions.connectNodes,
      discard,
      disconnectEdge: mutationActions.disconnectEdge,
      addEdgeWaypoint: layoutDraftState.addEdgeWaypoint,
      moveEdgeWaypoint: layoutDraftState.moveEdgeWaypoint,
      removeEdgeWaypoint: layoutDraftState.removeEdgeWaypoint,
      moveLayoutNode: layoutDraftState.moveNode,
      moveLayoutNodesByDelta: layoutDraftState.moveNodesByDelta,
      removeNode: mutationActions.removeNode,
      removeSelection: mutationActions.removeSelection,
      resetLayout: layoutDraftState.resetLayout,
      redoLayout: layoutDraftState.redoLayout,
      save: saveController.save,
      undoLayout: layoutDraftState.undoLayout,
      updateLayoutViewport: layoutDraftState.updateViewport,
      createVisualGroup: layoutDraftState.createVisualGroup,
      renameVisualGroup: layoutDraftState.renameVisualGroup,
      setVisualGroupColor: layoutDraftState.setVisualGroupColor,
      addNodeToVisualGroup: layoutDraftState.addNodeToVisualGroup,
      removeNodeFromVisualGroup: layoutDraftState.removeNodeFromVisualGroup,
      moveVisualGroupByDelta: layoutDraftState.moveVisualGroupByDelta,
      resizeVisualGroup: layoutDraftState.resizeVisualGroup,
      deleteVisualGroup: layoutDraftState.deleteVisualGroup,
      updateNodeField: mutationActions.updateNodeField,
    },
    blockedOperation,
    draftState,
    graphState,
    layoutDraftState,
    pendingState: {
      canRedoLayout: layoutDraftState.canRedoLayout,
      canUndoLayout: layoutDraftState.canUndoLayout,
      dirtyState,
      hasChanges: hasPortableDocumentChanges,
      hasLayoutChanges: dirtyState.layoutDirty,
      hasPortableDocumentChanges,
      hasPreferenceChanges: dirtyState.preferencesDirty,
      hasTopologyChanges: dirtyState.topologyDirty,
      layoutDirty: dirtyState.layoutDirty,
      pendingFactoryDefinition: draftState.pendingFactoryDefinition,
      preferencesDirty: dirtyState.preferencesDirty,
      topologyDirty: dirtyState.topologyDirty,
    },
    projection,
    documentSaveControls: saveController.documentSaveControls,
    saveMutation: saveController.saveMutation,
    saveState: {
      canSave: saveController.canSave,
      documentSave: saveController.documentSave,
      isStale: saveController.isStale,
    },
    validationState: {
      errors: draftState.validationErrors,
      isValid: draftState.validationErrors.length === 0,
    },
  };
}

function useEditableFactoryGraphState({
  baseFactoryDefinition,
  draftState,
  locale,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  locale?: string | null;
}) {
  return useMemo(
    () =>
      baseFactoryDefinition
        ? buildFactoryGraphState({
            baseFactoryDefinition,
            draft: draftState.draft,
            locale,
          })
        : null,
    [baseFactoryDefinition, draftState.draft, locale],
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: groups editable graph draft mutation actions behind one controller seam.
function useEditableFactoryGraphMutationActions({
  baseFactoryDefinition,
  draftState,
  layoutDraftState,
  locale,
  setBlockedOperation,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  layoutDraftState: ReturnType<typeof useFactoryGraphLayoutDraftState>;
  locale?: string | null;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  const applyDraftOperation = useCallback(
    (
      operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
    ): FactoryGraphOperationResult<FactoryGraphDraft> => {
      const result = operation();
      if (result.ok) {
        draftState.replaceDraft(result.value);
        setBlockedOperation(null);
      } else {
        setBlockedOperation(result as FactoryGraphOperationResult<never>);
      }
      return result;
    },
    [draftState, setBlockedOperation],
  );
  const addNode = useCallback(
    (node: FactoryGraphAddEntityDraft) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? addFactoryGraphNode({
              baseFactoryDefinition,
              draft: draftState.draft,
              locale,
              node,
            })
          : missingFactoryResult(
              "Load the current factory before editing graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draftState.draft, locale],
  );
  const removeNode = useCallback(
    (nodeId: string) => {
      const result = applyDraftOperation(() =>
        baseFactoryDefinition
          ? removeFactoryGraphNode({
              baseFactoryDefinition,
              draft: draftState.draft,
              locale,
              nodeId,
            })
          : missingFactoryResult(
              "Load the current factory before removing graph nodes.",
            ),
      );
      if (result.ok) {
        layoutDraftState.pruneLayoutHistoryForNodeIds(
          draftState.graph.nodes
            .filter((node) => node.id !== nodeId)
            .map((node) => node.id),
        );
      }
      return result;
    },
    [
      applyDraftOperation,
      baseFactoryDefinition,
      draftState.draft,
      draftState.graph.nodes,
      layoutDraftState,
      locale,
    ],
  );
  const connectNodes = useConnectNodesAction({
    applyDraftOperation,
    baseFactoryDefinition,
    draft: draftState.draft,
    locale,
  });
  const disconnectEdge = useDisconnectEdgeAction({
    applyDraftOperation,
    baseFactoryDefinition,
    draft: draftState.draft,
    locale,
  });
  const removeSelection = useCallback(
    (selection: { edgeIds: readonly string[]; nodeIds: readonly string[] }) => {
      const result = applyDraftOperation(() =>
        baseFactoryDefinition
          ? removeFactoryGraphSelection({
              baseFactoryDefinition,
              draft: draftState.draft,
              edgeIds: selection.edgeIds,
              nodeIds: selection.nodeIds,
              locale,
            })
          : missingFactoryResult(
              "Load the current factory before removing graph selection.",
            ),
      );
      if (result.ok) {
        layoutDraftState.pruneLayoutHistoryForNodeIds(
          draftState.graph.nodes
            .filter((node) => !selection.nodeIds.includes(node.id))
            .map((node) => node.id),
        );
      }
      return result;
    },
    [
      applyDraftOperation,
      baseFactoryDefinition,
      draftState.draft,
      draftState.graph.nodes,
      layoutDraftState,
      locale,
    ],
  );
  const updateNodeField = useUpdateNodeFieldAction({
    baseFactoryDefinition,
    setBlockedOperation,
  });

  return {
    addNode,
    connectNodes,
    disconnectEdge,
    removeNode,
    removeSelection,
    updateNodeField,
  };
}

function useConnectNodesAction({
  applyDraftOperation,
  baseFactoryDefinition,
  draft,
  locale,
}: {
  applyDraftOperation: (
    operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
  ) => FactoryGraphOperationResult<FactoryGraphDraft>;
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphDraft;
  locale?: string | null;
}) {
  return useCallback(
    (connection: {
      sourceAnchorId: string;
      sourceNodeId: string;
      targetAnchorId: string;
      targetNodeId: string;
    }) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? connectFactoryGraphNodes({
              baseFactoryDefinition,
              draft,
              locale,
              ...connection,
            })
          : missingFactoryResult(
              "Load the current factory before connecting graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draft, locale],
  );
}

function useDisconnectEdgeAction({
  applyDraftOperation,
  baseFactoryDefinition,
  draft,
  locale,
}: {
  applyDraftOperation: (
    operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
  ) => FactoryGraphOperationResult<FactoryGraphDraft>;
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphDraft;
  locale?: string | null;
}) {
  return useCallback(
    (edgeId: string) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? disconnectFactoryGraphEdge({
              baseFactoryDefinition,
              draft,
              edgeId,
              locale,
            })
          : missingFactoryResult(
              "Load the current factory before disconnecting graph edges.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draft, locale],
  );
}

function useUpdateNodeFieldAction({
  baseFactoryDefinition,
  setBlockedOperation,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  return useCallback(
    (update: FactoryGraphNodeFieldUpdate) => {
      if (!baseFactoryDefinition) {
        const result = missingFactoryResult(
          "Load the current factory before editing graph fields.",
        );
        setBlockedOperation(result);
        return result;
      }

      const result = updateFactoryGraphNodeField({
        baseFactoryDefinition,
        update,
      });
      setBlockedOperation(
        result.ok ? null : (result as FactoryGraphOperationResult<never>),
      );
      return result;
    },
    [baseFactoryDefinition, setBlockedOperation],
  );
}

function missingFactoryResult(
  message: string,
): FactoryGraphOperationResult<never> {
  return {
    message,
    ok: false,
    reason: "INVALID_SAVE",
  };
}
