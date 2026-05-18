import {
  applyNodeChanges,
  type FitViewOptions,
  type Node,
  type NodeChange,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { nodeKeyId } from "../factory-graph-editor/factory-graph-draft";
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
  const editorGraph = useMemo(
    () =>
      buildFactoryGraphEditorFlowModel({
        pendingNodeIds: collectPendingNodeIds(editor.draftState.draft),
        topology: editor.draftState.graph,
      }),
    [editor.draftState.draft, editor.draftState.graph],
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
