import {
  applyNodeChanges,
  type FitViewOptions,
  type Node,
  type NodeChange,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  buildFactoryGraphTopologyFromDefinition,
} from "../../factory-graph-editor/lib/factory-graph-draft-graph";
import {
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "../../factory-graph-editor/components/factory-graph-editor-flow";
import type { FactoryGraphEditorVisibilityPreset } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  collectPendingRemovalEdgeIds,
  collectPendingRemovalNodeIds,
} from "../../factory-graph-editor/lib/factory-graph-editor-removals";
import { buildFactoryGraphWorkerStatusMap } from "../../factory-graph-editor/lib/factory-graph-editor-runtime";
import {
  nodeKeyId,
  type FactoryGraphTopology,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import { useFactoryGraphEditorLayoutPositions } from "./react-flow-current-activity-card-editor-layout";
import { EMPTY_NODE_POSITIONS } from "../lib/react-flow-current-activity-card-graph";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";

export function useFactoryGraphEditorViewModel(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
  snapshot: DashboardSnapshot,
  locale?: string,
) {
  const {
    displayTopology,
    pendingAdditionEdgeIds,
    pendingRemovalEdgeIds,
    pendingRemovalNodeIds,
  } = useFactoryGraphEditorDraftGraphState(editor);
  const {
    visibilityPresetOptions,
    filteredTopology,
    filteredTopologyKey,
    selectVisibilityPreset,
  } = useFactoryGraphEntityVisibilityState(displayTopology, locale);
  const editorGraph = useFactoryGraphEditorFlowGraph({
    editor,
    filteredTopology,
    filteredTopologyKey,
    locale,
    pendingAdditionEdgeIds,
    pendingRemovalEdgeIds,
    pendingRemovalNodeIds,
    snapshot,
  });
  const graphKey = useMemo(
    () => ["factory-editor", filteredTopologyKey].join(":"),
    [filteredTopologyKey],
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
    edgeTypes: FACTORY_GRAPH_EDITOR_EDGE_TYPES,
    visibilityPresetOptions,
    nodeTypes: FACTORY_GRAPH_EDITOR_NODE_TYPES,
    nodes,
    setStoredNodePosition,
    selectVisibilityPreset,
  };
}

function useFactoryGraphEntityVisibilityState(
  displayTopology: FactoryGraphTopology,
  locale?: string,
) {
  const messages = getFactoryGraphEditorMessages(locale);
  const [selectedPreset, setSelectedPreset] =
    useState<FactoryGraphEditorVisibilityPreset>("all");
  const filteredTopology = useMemo(
    () => filterFactoryGraphTopology(displayTopology, selectedPreset),
    [displayTopology, selectedPreset],
  );
  const visibilityPresetOptions = useMemo(
    () => [
      {
        key: "all" as const,
        label: messages.visibilityPresetAllLabel,
        selected: selectedPreset === "all",
      },
      {
        key: "workflow" as const,
        label: messages.visibilityPresetWorkflowLabel,
        selected: selectedPreset === "workflow",
      },
      {
        key: "execution" as const,
        label: messages.visibilityPresetExecutionLabel,
        selected: selectedPreset === "execution",
      },
      {
        key: "infrastructure" as const,
        label: messages.visibilityPresetInfrastructureLabel,
        selected: selectedPreset === "infrastructure",
      },
    ],
    [
      messages.visibilityPresetAllLabel,
      messages.visibilityPresetExecutionLabel,
      messages.visibilityPresetInfrastructureLabel,
      messages.visibilityPresetWorkflowLabel,
      selectedPreset,
    ],
  );
  const selectVisibilityPreset = useCallback(
    (preset: FactoryGraphEditorVisibilityPreset) => setSelectedPreset(preset),
    [],
  );

  return {
    visibilityPresetOptions,
    filteredTopology,
    filteredTopologyKey: graphTopologyKey(filteredTopology, selectedPreset),
    selectVisibilityPreset,
  };
}

function useFactoryGraphEditorFlowGraph(input: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  filteredTopology: FactoryGraphTopology;
  filteredTopologyKey: string;
  locale?: string;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  pendingRemovalEdgeIds: ReadonlySet<string>;
  pendingRemovalNodeIds: ReadonlySet<string>;
  snapshot: DashboardSnapshot;
}) {
  const layoutPositionsByNodeId = useFactoryGraphEditorLayoutPositions(
    input.filteredTopology,
    input.filteredTopologyKey,
  );
  const workerStatusByName = useMemo(
    () =>
      buildFactoryGraphWorkerStatusMap({
        factoryDefinition:
          input.editor.draftState.pendingFactoryDefinition ??
          input.editor.draftState.latestDocument ??
          null,
        snapshot: input.snapshot,
      }),
    [
      input.editor.draftState.latestDocument,
      input.editor.draftState.pendingFactoryDefinition,
      input.snapshot,
    ],
  );

  return useMemo(
    () =>
      buildFactoryGraphEditorFlowModel({
        canEditConnections: input.editor.activeTool === "connect",
        layoutPositionsByNodeId,
        locale: input.locale,
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
      layoutPositionsByNodeId,
      input.locale,
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
            editor.draftState.latestDocument,
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
            editor.draftState.latestDocument,
            editor.draftState.draft,
          )
        : new Set<string>(),
    [editor.draftState.draft, editor.draftState.latestDocument],
  );
  const pendingRemovalEdgeIds = useMemo(
    () =>
      editor.draftState.latestDocument
        ? collectPendingRemovalEdgeIds(
            editor.draftState.latestDocument,
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

function graphTopologyKey(
  topology: FactoryGraphTopology,
  selectedPreset?: FactoryGraphEditorVisibilityPreset,
) {
  return [
    selectedPreset ?? "all",
    ...topology.nodes.map((node) => node.id),
    ...topology.edges.map((edge) => edge.id),
  ].join(":");
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

function filterFactoryGraphTopology(
  topology: FactoryGraphTopology,
  preset: FactoryGraphEditorVisibilityPreset,
) {
  if (preset === "all") {
    return topology;
  }

  const { visibleEdgeKinds, visibleNodeKinds } =
    FACTORY_GRAPH_PRESET_VISIBILITY[preset];

  const visibleNodes = topology.nodes.filter(
    (node) => visibleNodeKinds.has(node.kind),
  );
  const visibleNodeIds = new Set(visibleNodes.map((node) => node.id));
  return {
    edges: topology.edges.filter(
      (edge) =>
        visibleEdgeKinds.has(edge.kind) &&
        visibleNodeIds.has(edge.sourceId) && visibleNodeIds.has(edge.targetId),
    ),
    nodes: visibleNodes,
  };
}

const FACTORY_GRAPH_PRESET_VISIBILITY: Record<
  Exclude<FactoryGraphEditorVisibilityPreset, "all">,
  {
    visibleEdgeKinds: ReadonlySet<FactoryGraphTopology["edges"][number]["kind"]>;
    visibleNodeKinds: ReadonlySet<FactoryGraphTopology["nodes"][number]["kind"]>;
  }
> = {
  execution: {
    visibleEdgeKinds: new Set([
      "work-type-state",
      "workstation-input",
      "workstation-output",
      "workstation-on-continue",
      "workstation-on-failure",
      "workstation-on-rejection",
    ]),
    visibleNodeKinds: new Set(["work-state", "workstation"]),
  },
  infrastructure: {
    visibleEdgeKinds: new Set([
      "worker-assignment",
      "worker-resource",
      "workstation-resource",
    ]),
    visibleNodeKinds: new Set(["resource", "worker", "workstation"]),
  },
  workflow: {
    visibleEdgeKinds: new Set([
      "work-type-state",
      "workstation-input",
      "workstation-output",
    ]),
    visibleNodeKinds: new Set(["work-state", "work-type", "workstation"]),
  },
};
