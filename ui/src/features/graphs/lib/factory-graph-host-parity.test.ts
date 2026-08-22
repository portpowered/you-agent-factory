import {
  assertFactoryGraphHostParity,
  FACTORY_GRAPH_HOST_PARITY_HOSTS,
  FACTORY_GRAPH_NODE_TYPES,
  type FactoryGraphHostParityProjection,
  type FactoryGraphNodeFamily,
  factoryGraphNodeFamilyRole,
  projectFactoryGraphHostParity,
  projectFactoryGraphReplayFlow,
} from "@you-agent-factory/factory-graph";
import { projectFactoryTopology } from "@you-agent-factory/factory-replay";
import { describe, expect, it } from "vitest";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "../../factory-graph-editor/components/flow/factory-graph-editor-flow";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-graph";
import { projectFactoryGraphToReactFlow } from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection";
import { NODE_TYPES } from "../../flowchart/components/current-activity-nodes";
import { buildTraceDispatchFactoryGraphFlow } from "../../trace-drilldown/lib/trace-dispatch-factory-graph-flow";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "../../workflow-activity/lib/current-activity-factory-graph-layout";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildVisibleGraphEdges,
} from "../../workflow-activity/lib/react-flow-current-activity-card-graph";

const PARITY_NOW = Date.parse("2026-08-14T20:00:00Z");
const PARITY_TICK = 4;
const FULL_PARITY_FIELDS = [
  "dimensions",
  "family",
  "handles",
  "semanticNodeId",
  "type",
  "visual",
  "workProgress",
  "workstationSemantics",
] as const;
const TRACE_PARITY_FIELDS = [
  "dimensions",
  "family",
  "handles",
  "semanticNodeId",
  "type",
  "visual",
  "workProgress",
  "workstationSemantics",
] as const;

describe("Factory graph host parity", () => {
  it("compares current activity, editor, replay, and Factory-semantic trace hosts", async () => {
    const fixture = await buildParityFixture();
    const hosts = fixture.hosts;

    expect(FACTORY_GRAPH_HOST_PARITY_HOSTS).toEqual([
      "current-activity",
      "editor",
      "replay",
      "trace",
    ]);
    expect(Object.keys(hosts)).toEqual(
      expect.arrayContaining(FACTORY_GRAPH_HOST_PARITY_HOSTS),
    );

    assertFactoryGraphHostParity({
      comparisons: [
        {
          fields: FULL_PARITY_FIELDS,
          hosts: ["current-activity", "editor", "replay"],
          nodeIds: fixture.topology.nodes.map((node) => node.id),
        },
      ],
      hosts: {
        "current-activity": hosts["current-activity"],
        editor: hosts.editor,
        replay: hosts.replay,
      },
    });

    assertFactoryGraphHostParity({
      comparisons: [
        {
          fields: TRACE_PARITY_FIELDS,
          hosts: ["trace", "trace-reference"],
          nodeIds: ["workstation:review"],
        },
      ],
      hosts: {
        trace: hosts.trace,
        "trace-reference": hosts["trace-reference"],
      },
    });
  });

  it("fails when a host introduces a local measurement override", async () => {
    const fixture = await buildParityFixture();
    const editor = fixture.hosts.editor;
    const workstation = editor.nodes.find(
      (node) => node.semanticNodeId === "workstation:draft",
    );
    if (!workstation) throw new Error("parity fixture lacks draft workstation");

    const overriddenEditor = {
      ...editor,
      nodes: editor.nodes.map((node) =>
        node === workstation
          ? {
              ...node,
              dimensions: {
                ...node.dimensions,
                width: (node.dimensions.width ?? 0) + 1,
              },
            }
          : node,
      ),
    } satisfies FactoryGraphHostParityProjection;

    expect(() =>
      assertFactoryGraphHostParity({
        comparisons: [
          {
            fields: FULL_PARITY_FIELDS,
            hosts: ["current-activity", "editor", "replay"],
            nodeIds: fixture.topology.nodes.map((node) => node.id),
          },
        ],
        hosts: {
          "current-activity": fixture.hosts["current-activity"],
          editor: overriddenEditor,
          replay: fixture.hosts.replay,
        },
      }),
    ).toThrow(
      "[factory-graph-parity] editor diverges from current-activity for workstation:draft.dimensions",
    );
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: one canonical fixture wires each real host projection into the shared guard.
async function buildParityFixture() {
  const factory = parityFactoryDefinition();
  const topology = buildFactoryGraphTopologyFromDefinition(factory);
  const factoryWithLayout = withParityLayout(
    factory,
    topology.nodes.map((node) => node.id),
  );
  const dashboardWorkstations = (factory.workstations ?? []).map(
    dashboardWorkstationFromFactory,
  );
  const snapshot = {
    factory: factoryWithLayout,
    factory_state: "IDLE",
    runtime: {
      active_executions_by_dispatch_id: {},
      current_work_items_by_place_id: {},
      in_flight_dispatch_count: 0,
      place_occupancy_work_items_by_place_id: {},
      place_token_counts: { "story:queued": 4 },
      session: {
        completed_count: 0,
        dispatched_count: 0,
        failed_count: 0,
        has_data: true,
        provider_sessions: [],
      },
      workstation_requests_by_dispatch_id: {},
    },
    tick_count: PARITY_TICK,
    topology: {
      edges: [],
      workstation_node_ids: dashboardWorkstations.map(
        (workstation) => workstation.node_id,
      ),
      workstation_nodes_by_id: Object.fromEntries(
        dashboardWorkstations.map((workstation) => [
          workstation.node_id,
          workstation,
        ]),
      ),
    },
    uptime_seconds: 0,
  } satisfies DashboardSnapshot;

  const graphLayout =
    await buildCurrentActivityGraphLayoutFromFactory(factoryWithLayout);
  const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
  const currentNodes = buildCurrentActivityNodes({
    activeExecutionsByWorkstationNodeID: {},
    activeGraphHighlights: buildActiveGraphHighlights(
      [],
      visibleGraphEdges,
      graphLayout.nodes,
    ),
    activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
    factoryDefinition: factoryWithLayout,
    graphLayout,
    now: PARITY_NOW,
    onSelectDoc: () => undefined,
    onSelectResource: () => undefined,
    onSelectStateNode: () => undefined,
    onSelectWorkID: () => undefined,
    onSelectWorker: () => undefined,
    onSelectWorkType: () => undefined,
    onSelectWorkstation: () => undefined,
    selection: null,
    snapshot,
  });

  const editorFlow = buildFactoryGraphEditorFlowModel({
    canEditConnections: false,
    factoryDefinition: factoryWithLayout,
    layout: factoryWithLayout.layout,
    pendingAdditionEdgeIds: new Set(),
    pendingAdditionNodeIds: new Set(),
    pendingConnectionSource: null,
    pendingRemovalEdgeIds: new Set(),
    pendingRemovalNodeIds: new Set(),
    placeTokenCountsByNodeId: new Map([["work-state:story:queued", 4]]),
    topology,
  });
  const replayTopology = projectFactoryTopology({
    factory: factoryWithLayout,
    selectedTick: PARITY_TICK,
  });
  const replay = projectFactoryGraphReplayFlow({
    factory: factoryWithLayout,
    runtime: {
      activity: {
        activeDispatchOverlays: [],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: PARITY_TICK,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: PARITY_TICK,
        workStateCounts: [
          {
            count: 4,
            evidence: "known",
            workStateId: "queued",
            workStateNodeId: "work-state:story:queued",
            workTypeId: "story",
          },
        ],
      },
      topology: replayTopology,
    },
    selectedTick: PARITY_TICK,
  });
  const traceFlow = buildTraceDispatchFactoryGraphFlow([
    {
      dispatch_id: "dispatch-review",
      duration_millis: 1000,
      end_time: "2026-08-14T20:00:01Z",
      outcome: "UNKNOWN",
      start_time: "2026-08-14T20:00:00Z",
      transition_id: "review",
      workstation_name: "review",
    },
  ]);
  const traceReference = projectFactoryGraphToReactFlow({
    mode: "observer",
    topology: traceFlow.topology,
  });
  const parityGroups = factoryWithLayout.layout?.groups;

  return {
    hosts: {
      "current-activity": projectFactoryGraphHostParity({
        groups: parityGroups,
        host: "current-activity",
        nodeTypes: NODE_TYPES,
        nodes: currentNodes,
      }),
      editor: projectFactoryGraphHostParity({
        groups: parityGroups,
        host: "editor",
        nodeTypes: FACTORY_GRAPH_EDITOR_NODE_TYPES,
        nodes: editorFlow.nodes,
      }),
      replay: projectFactoryGraphHostParity({
        groups: parityGroups,
        host: "replay",
        nodeTypes: FACTORY_GRAPH_NODE_TYPES,
        nodes: replay.nodes,
      }),
      trace: projectFactoryGraphHostParity({
        host: "trace",
        nodeTypes: FACTORY_GRAPH_NODE_TYPES,
        nodes: traceFlow.nodes,
      }),
      "trace-reference": projectFactoryGraphHostParity({
        host: "trace-reference",
        nodeTypes: FACTORY_GRAPH_NODE_TYPES,
        nodes: traceReference.nodes,
      }),
    },
    topology,
  };
}

function parityFactoryDefinition(): CanonicalFactoryDefinition {
  const workstation = baseFactoryDefinition.workstations?.[0];
  if (!workstation) throw new Error("parity fixture lacks workstation");

  return {
    ...baseFactoryDefinition,
    workstations: [
      {
        ...workstation,
        onContinue: [{ state: "queued", workType: "story" }],
        onFailure: [{ state: "queued", workType: "story" }],
        onRejection: [{ state: "queued", workType: "story" }],
      },
    ],
  };
}

function withParityLayout(
  factory: CanonicalFactoryDefinition,
  nodeIds: readonly string[],
): CanonicalFactoryDefinition {
  const layoutNodes = nodeIds.map((id, index) => {
    const family: FactoryGraphNodeFamily = id.startsWith("work-state:")
      ? "work-state"
      : id.startsWith("workstation:")
        ? "workstation"
        : id.startsWith("worker:")
          ? "worker"
          : id.startsWith("work-type:")
            ? "work-type"
            : "constraint";
    const role = factoryGraphNodeFamilyRole(family);
    return {
      id,
      position: { x: index * 240, y: index * 36 },
      size: {
        height: Math.min(
          role.maximumDimensions.height,
          role.defaultDimensions.height + (index % 2) * 12,
        ),
        width: Math.min(
          role.maximumDimensions.width,
          role.defaultDimensions.width + (index % 3) * 16,
        ),
      },
    };
  });
  return {
    ...factory,
    layout: {
      groups: [
        {
          bounds: { height: 560, width: 1320, x: -80, y: -80 },
          color: "info",
          id: "workflow",
          label: "Workflow",
          nodeIds: [...nodeIds],
        },
      ],
      nodes: layoutNodes,
      schemaVersion: 1,
      viewport: { x: 16, y: 24, zoom: 0.9 },
    },
  };
}
