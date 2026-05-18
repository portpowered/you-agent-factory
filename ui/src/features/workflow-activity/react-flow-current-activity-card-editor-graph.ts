import {
  applyNodeChanges,
  type FitViewOptions,
  type Node,
  type NodeChange,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { DashboardSnapshot } from "../../api/dashboard/types";
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
import { buildFactoryGraphWorkerStatusMap } from "../factory-graph-editor/factory-graph-editor-runtime";
import type { FactoryGraphTopology } from "../factory-graph-editor/factory-graph-draft-types";
import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import { EMPTY_NODE_POSITIONS } from "./react-flow-current-activity-card-graph";
import { useCurrentActivityGraphStore } from "./state/currentActivityGraphStore";

export function useFactoryGraphEditorViewModel(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
  snapshot: DashboardSnapshot,
) {
  const {
    displayTopology,
    pendingAdditionEdgeIds,
    pendingRemovalEdgeIds,
    pendingRemovalNodeIds,
  } = useFactoryGraphEditorDraftGraphState(editor);
  const {
    entityVisibilityOptions,
    filteredTopology,
    toggleEntityVisibility,
  } = useFactoryGraphEntityVisibilityState(displayTopology);
  const editorGraph = useFactoryGraphEditorFlowGraph({
    editor,
    filteredTopology,
    pendingAdditionEdgeIds,
    pendingRemovalEdgeIds,
    pendingRemovalNodeIds,
    snapshot,
  });
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
  const { handleNodesChange, nodes } = useFactoryGraphEditorPersistentNodes({
    editorNodes: editorGraph.nodes,
    storedNodePositions,
  });

  return {
    edges: editorGraph.edges,
    graphKey,
    handleNodesChange,
    initialFitViewKey: graphKey,
    initialFitViewOptions: {
      maxZoom: 1.1,
      padding: 0.18,
    } satisfies FitViewOptions,
    entityVisibilityOptions,
    nodeTypes: FACTORY_GRAPH_EDITOR_NODE_TYPES,
    nodes,
    setStoredNodePosition,
    toggleEntityVisibility,
  };
}

function useFactoryGraphEntityVisibilityState(displayTopology: FactoryGraphTopology) {
  const [visibleEntityKinds, setVisibleEntityKinds] = useState({
    resources: true,
    workers: true,
  });
  const filteredTopology = useMemo(
    () => filterFactoryGraphTopology(displayTopology, visibleEntityKinds),
    [displayTopology, visibleEntityKinds],
  );
  const entityVisibilityOptions = useMemo(
    () => [
      {
        count: countNodesByKind(displayTopology, "resource"),
        key: "resources" as const,
        label: "Resources",
        visible: visibleEntityKinds.resources,
      },
      {
        count: countNodesByKind(displayTopology, "worker"),
        key: "workers" as const,
        label: "Workers",
        visible: visibleEntityKinds.workers,
      },
    ],
    [displayTopology, visibleEntityKinds.resources, visibleEntityKinds.workers],
  );
  const toggleEntityVisibility = useCallback(
    (key: "resources" | "workers") =>
      setVisibleEntityKinds((currentVisibility) => ({
        ...currentVisibility,
        [key]: !currentVisibility[key],
      })),
    [],
  );

  return {
    entityVisibilityOptions,
    filteredTopology,
    toggleEntityVisibility,
  };
}

function useFactoryGraphEditorFlowGraph(input: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  filteredTopology: FactoryGraphTopology;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  pendingRemovalEdgeIds: ReadonlySet<string>;
  pendingRemovalNodeIds: ReadonlySet<string>;
  snapshot: DashboardSnapshot;
}) {
  const workerStatusByName = useMemo(
    () =>
      buildFactoryGraphWorkerStatusMap({
        factoryDefinition:
          input.editor.draftState.pendingFactoryDefinition ??
          input.editor.draftState.latestDocument?.factoryDefinition ??
          null,
        snapshot: input.snapshot,
      }),
    [
      input.editor.draftState.latestDocument?.factoryDefinition,
      input.editor.draftState.pendingFactoryDefinition,
      input.snapshot,
    ],
  );

  return useMemo(
    () =>
      buildFactoryGraphEditorFlowModel({
        canEditConnections: input.editor.activeTool === "connect",
        onConnectionAnchorClick: input.editor.handleConnectionAnchorClick,
        pendingAdditionEdgeIds: input.pendingAdditionEdgeIds,
        pendingConnectionSource: input.editor.pendingConnectionSource,
        pendingAdditionNodeIds: collectPendingNodeIds(input.editor.draftState.draft),
        pendingRemovalEdgeIds: input.pendingRemovalEdgeIds,
        pendingRemovalNodeIds: input.pendingRemovalNodeIds,
        topology: input.filteredTopology,
        workerStatusByName,
      }),
    [
      input.filteredTopology,
      input.editor.activeTool,
      input.editor.draftState.draft,
      input.editor.handleConnectionAnchorClick,
      input.editor.pendingConnectionSource,
      input.pendingAdditionEdgeIds,
      input.pendingRemovalEdgeIds,
      input.pendingRemovalNodeIds,
      workerStatusByName,
    ],
  );
}

function useFactoryGraphEditorPersistentNodes(input: {
  editorNodes: Node[];
  storedNodePositions: typeof EMPTY_NODE_POSITIONS;
}) {
  const baseNodes = useMemo<Node[]>(
    () =>
      input.editorNodes.map((node) => ({
        ...node,
        position: input.storedNodePositions[node.id] ?? node.position,
      })),
    [input.editorNodes, input.storedNodePositions],
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

  return { handleNodesChange, nodes };
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

function countNodesByKind(
  topology: FactoryGraphTopology,
  kind: FactoryGraphTopology["nodes"][number]["kind"],
) {
  return topology.nodes.filter((node) => node.kind === kind).length;
}

function filterFactoryGraphTopology(
  topology: FactoryGraphTopology,
  visibility: {
    resources: boolean;
    workers: boolean;
  },
) {
  const hiddenNodeKinds = new Set<FactoryGraphTopology["nodes"][number]["kind"]>();
  if (!visibility.resources) {
    hiddenNodeKinds.add("resource");
  }
  if (!visibility.workers) {
    hiddenNodeKinds.add("worker");
  }
  if (hiddenNodeKinds.size === 0) {
    return topology;
  }

  const visibleNodes = topology.nodes.filter(
    (node) => !hiddenNodeKinds.has(node.kind),
  );
  const visibleNodeIds = new Set(visibleNodes.map((node) => node.id));
  return {
    edges: topology.edges.filter(
      (edge) =>
        visibleNodeIds.has(edge.sourceId) && visibleNodeIds.has(edge.targetId),
    ),
    nodes: visibleNodes,
  };
}
