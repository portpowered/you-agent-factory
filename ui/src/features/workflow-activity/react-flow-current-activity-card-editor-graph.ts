import {
  applyNodeChanges,
  type FitViewOptions,
  type Node,
  type NodeChange,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import {
  buildFactoryGraphTopologyFromDefinition,
  collectPendingRemovalEdgeIds,
  collectPendingRemovalNodeIds,
  nodeKeyId,
} from "../factory-graph-editor/factory-graph-draft";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "../factory-graph-editor/factory-graph-editor-flow";
import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import { EMPTY_NODE_POSITIONS } from "./react-flow-current-activity-card-graph";
import { useCurrentActivityGraphStore } from "./state/currentActivityGraphStore";

export function useFactoryGraphEditorViewModel(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  const {
    displayTopology,
    pendingAdditionEdgeIds,
    pendingRemovalEdgeIds,
    pendingRemovalNodeIds,
  } = useFactoryGraphEditorDraftGraphState(editor);
  const editorGraph = useMemo(
    () =>
      buildFactoryGraphEditorFlowModel({
        canEditConnections: editor.activeTool === "connect",
        onConnectionAnchorClick: editor.handleConnectionAnchorClick,
        pendingAdditionEdgeIds,
        pendingConnectionSource: editor.pendingConnectionSource,
        pendingAdditionNodeIds: collectPendingNodeIds(editor.draftState.draft),
        pendingRemovalEdgeIds,
        pendingRemovalNodeIds,
        topology: displayTopology,
      }),
    [
      displayTopology,
      editor.draftState.draft,
      editor.activeTool,
      editor.handleConnectionAnchorClick,
      pendingAdditionEdgeIds,
      editor.pendingConnectionSource,
      pendingRemovalEdgeIds,
      pendingRemovalNodeIds,
    ],
  );
  const graphKey = useMemo(
    () =>
      [
        "factory-editor",
        ...editorGraph.nodes.map((node) => node.id),
        ...editorGraph.edges.map((edge) => edge.id),
      ].join(":"),
    [editorGraph.edges, editorGraph.nodes],
  );
  const storedNodePositions = useCurrentActivityGraphStore(
    (state) => state.positionsByGraphKey[graphKey] ?? EMPTY_NODE_POSITIONS,
  );
  const setStoredNodePosition = useCurrentActivityGraphStore(
    (state) => state.setNodePosition,
  );
  const baseNodes = useMemo<Node[]>(
    () =>
      editorGraph.nodes.map((node) => ({
        ...node,
        position: storedNodePositions[node.id] ?? node.position,
      })),
    [editorGraph.nodes, storedNodePositions],
  );
  const [nodes, setNodes] = useState<Node[]>([]);

  useEffect(() => {
    setNodes((currentNodes) => {
      const currentPositions = new Map(
        currentNodes.map((node) => [node.id, node.position]),
      );
      return baseNodes.map((node) => ({
        ...node,
        position: currentPositions.get(node.id) ?? node.position,
      }));
    });
  }, [baseNodes]);

  const handleNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes((currentNodes) => applyNodeChanges(changes, currentNodes) as Node[]);
  }, []);

  return {
    edges: editorGraph.edges,
    graphKey,
    handleNodesChange,
    initialFitViewKey: graphKey,
    initialFitViewOptions: {
      maxZoom: 1.1,
      padding: 0.18,
    } satisfies FitViewOptions,
    nodeTypes: FACTORY_GRAPH_EDITOR_NODE_TYPES,
    nodes,
    setStoredNodePosition,
  };
}

function useFactoryGraphEditorDraftGraphState(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  const baseTopology = useMemo(
    () =>
      editor.draftState.latestDocument
        ? buildFactoryGraphTopologyFromDefinition(
            editor.draftState.latestDocument.factoryDefinition,
          )
        : { edges: [], nodes: [] },
    [editor.draftState.latestDocument],
  );
  const displayTopology = useMemo(
    () => ({
      edges: mergeById(baseTopology.edges, editor.draftState.graph.edges),
      nodes: mergeById(baseTopology.nodes, editor.draftState.graph.nodes),
    }),
    [
      baseTopology.edges,
      baseTopology.nodes,
      editor.draftState.graph.edges,
      editor.draftState.graph.nodes,
    ],
  );
  const pendingRemovalNodeIds = useMemo(
    () =>
      editor.draftState.latestDocument
        ? collectPendingRemovalNodeIds(
            editor.draftState.latestDocument.factoryDefinition,
            editor.draftState.draft,
          )
        : new Set<string>(),
    [editor.draftState.draft, editor.draftState.latestDocument],
  );
  const pendingRemovalEdgeIds = useMemo(
    () =>
      editor.draftState.latestDocument
        ? collectPendingRemovalEdgeIds(
            editor.draftState.latestDocument.factoryDefinition,
            editor.draftState.draft,
          )
        : new Set<string>(),
    [editor.draftState.draft, editor.draftState.latestDocument],
  );
  const pendingAdditionEdgeIds = useMemo(
    () =>
      new Set(
        editor.draftState.draft.edgeChanges.additions.map((edge) =>
          `${edge.kind}:${nodeKeyId(edge.source)}->${nodeKeyId(edge.target)}`,
        ),
      ),
    [editor.draftState.draft.edgeChanges.additions],
  );

  return {
    displayTopology,
    pendingAdditionEdgeIds,
    pendingRemovalEdgeIds,
    pendingRemovalNodeIds,
  };
}

function mergeById<T extends { id: string }>(baseItems: T[], nextItems: T[]) {
  const itemsById = new Map<string, T>();

  for (const item of baseItems) {
    itemsById.set(item.id, item);
  }
  for (const item of nextItems) {
    itemsById.set(item.id, item);
  }

  return Array.from(itemsById.values()).sort((left, right) =>
    left.id.localeCompare(right.id),
  );
}

function collectPendingNodeIds(
  draftState: ReturnType<typeof useCurrentActivityGraphEditor>["draftState"]["draft"],
) {
  const pendingNodeIds = new Set<string>();

  for (const resource of draftState.additions.resources) {
    pendingNodeIds.add(
      nodeKeyId({
        kind: "resource",
        name: resource.name,
      }),
    );
  }
  for (const worker of draftState.additions.workers) {
    pendingNodeIds.add(
      nodeKeyId({
        kind: "worker",
        name: worker.name,
      }),
    );
  }
  for (const workType of draftState.additions.workTypes) {
    pendingNodeIds.add(
      nodeKeyId({
        kind: "work-type",
        name: workType.name,
      }),
    );
  }
  for (const workState of draftState.additions.workStates) {
    pendingNodeIds.add(
      nodeKeyId({
        kind: "work-state",
        stateName: workState.state.name,
        workTypeName: workState.workTypeName,
      }),
    );
  }
  for (const workstation of draftState.additions.workstations) {
    pendingNodeIds.add(
      nodeKeyId({
        kind: "workstation",
        name: workstation.name,
      }),
    );
  }

  return pendingNodeIds;
}
