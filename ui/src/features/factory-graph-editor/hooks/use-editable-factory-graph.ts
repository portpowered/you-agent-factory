import { useCallback, useMemo, useState } from "react";

import { resolveProjectedLayoutPositions } from "../lib/factory-graph-layout-operations";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
} from "../lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import { createFactoryGraphWorkstationResolver } from "../lib/factory-graph-editor-connections";
import type { FactoryGraphNodeFieldUpdate } from "../lib/factory-graph-field-operations";
import { updateFactoryGraphNodeField } from "../lib/factory-graph-field-operations";
import {
  addFactoryGraphNode,
  buildFactoryGraphState,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
  type FactoryGraphOperationResult,
  projectFactoryGraphToReactFlow,
  removeFactoryGraphNode,
} from "../lib/factory-graph-operations";
import { useFactoryGraphDraftState } from "./factory-graph-draft-hook";
import { useFactoryGraphLayoutDraftState } from "./factory-graph-layout-draft-hook";
import { useEditableFactoryGraphSaveController } from "./use-editable-factory-graph-save-controller";
import type {
  EditableFactoryGraphViewModel,
  UseEditableFactoryGraphOptions,
} from "./use-editable-factory-graph-types";

export function useEditableFactoryGraph(
  options: UseEditableFactoryGraphOptions,
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
    locale: options.locale,
    setBlockedOperation,
  });
  const discard = useCallback(() => {
    draftState.resetDraft();
    layoutDraftState.resetLayout();
    setBlockedOperation(null);
    saveController.resetSaveState();
  }, [draftState, layoutDraftState, saveController]);

  return {
    actions: {
      addNode: mutationActions.addNode,
      connectNodes: mutationActions.connectNodes,
      discard,
      disconnectEdge: mutationActions.disconnectEdge,
      moveLayoutNode: layoutDraftState.moveNode,
      removeNode: mutationActions.removeNode,
      save: saveController.save,
      updateNodeField: mutationActions.updateNodeField,
    },
    blockedOperation,
    draftState,
    graphState,
    layoutDraftState,
    pendingState: {
      hasChanges: draftState.hasChanges || layoutDraftState.hasChanges,
      hasLayoutChanges: layoutDraftState.hasChanges,
      hasTopologyChanges: draftState.hasChanges,
      pendingFactoryDefinition: draftState.pendingFactoryDefinition,
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

function useEditableFactoryGraphMutationActions({
  baseFactoryDefinition,
  draftState,
  locale,
  setBlockedOperation,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
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
    (nodeId: string) =>
      applyDraftOperation(() =>
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
      ),
    [applyDraftOperation, baseFactoryDefinition, draftState.draft, locale],
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
  const updateNodeField = useUpdateNodeFieldAction({
    baseFactoryDefinition,
    setBlockedOperation,
  });

  return {
    addNode,
    connectNodes,
    disconnectEdge,
    removeNode,
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
