import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft.test-helpers";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "./current-activity-factory-graph-layout";
import { factoryGraphConnectionAnchorContext } from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import {
  buildEditorHandles,
  resolveWorkstationConnectionAnchorContext,
} from "./react-flow-current-activity-card-editor-handles";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildVisibleGraphEdges,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";

const logicalMoveContext = factoryGraphConnectionAnchorContext({
  type: "LOGICAL_MOVE",
});

const modelWorkstationContext = factoryGraphConnectionAnchorContext({
  type: "MODEL_WORKSTATION",
  behavior: "STANDARD",
});

const factoryWithLogicalMove = {
  ...baseFactoryDefinition,
  workstations: [
    ...(baseFactoryDefinition.workstations ?? []),
    {
      body: "Move work downstream.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "router",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "LOGICAL_MOVE",
      worker: "",
    },
  ],
} satisfies CanonicalFactoryDefinition;

function handleIds(handles: { id: string }[]) {
  return handles.map((handle) => handle.id);
}

it("omits worker-assignment-target on LOGICAL_MOVE workstations", () => {
  const ids = handleIds(
    buildEditorHandles({
      connectionAnchorContext: logicalMoveContext,
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: () => {},
        pendingConnectionSource: null,
      },
      nodeId: "workstation:router",
      nodeKind: "workstation",
    }),
  );

  expect(ids).not.toContain("worker-assignment-target");
  expect(ids).toEqual(
    expect.arrayContaining([
      "workstation-input-target",
      "workstation-resource-target",
      "workstation-output-source",
    ]),
  );
});

it("exposes worker-assignment-target on worker-backed workstations", () => {
  const ids = handleIds(
    buildEditorHandles({
      connectionAnchorContext: modelWorkstationContext,
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: () => {},
        pendingConnectionSource: null,
      },
      nodeId: "workstation:draft",
      nodeKind: "workstation",
    }),
  );

  expect(ids).toContain("worker-assignment-target");
});

it("resolves workstation context from factory definitions for handle filtering", () => {
  expect(
    resolveWorkstationConnectionAnchorContext(
      factoryWithLogicalMove,
      "workstation:router",
    )?.workstation.type,
  ).toBe("LOGICAL_MOVE");
  expect(
    resolveWorkstationConnectionAnchorContext(
      factoryWithLogicalMove,
      "workstation:draft",
    )?.workstation.type,
  ).toBe("MODEL_WORKSTATION");
});

it("builds current-activity workstation nodes without worker-assignment handles on logical-move stations", async () => {
  const graphLayout =
    await buildCurrentActivityGraphLayoutFromFactory(factoryWithLogicalMove);
  const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
  const workstations = (factoryWithLogicalMove.workstations ?? []).map(
    dashboardWorkstationFromFactory,
  );
  const nodes = buildCurrentActivityNodes({
    activeExecutionsByWorkstationNodeID: {},
    activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
    activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
    editor: {
      activeTool: "connect",
      canInteractWithEditor: true,
      editorMode: true,
      onConnectionAnchorClick: () => {},
      pendingConnectionSource: null,
    },
    factoryDefinition: factoryWithLogicalMove,
    graphLayout,
    now: Date.parse("2026-05-24T00:00:00Z"),
    onSelectStateNode: () => {},
    onSelectWorkID: () => {},
    onSelectWorker: () => {},
    onSelectWorkstation: () => {},
    selection: null,
    snapshot: {
      factory: factoryWithLogicalMove,
      factory_state: "IDLE",
      runtime: { place_token_counts: {} },
      tick_count: 0,
      topology: {
        edges: [],
        workstation_node_ids: workstations.map(
          (workstation) => workstation.node_id,
        ),
        workstation_nodes_by_id: Object.fromEntries(
          workstations.map((workstation) => [workstation.node_id, workstation]),
        ),
      },
      uptime_seconds: 0,
    },
    storedNodePositions: EMPTY_NODE_POSITIONS,
  });

  const logicalMoveHandles = handleIds(
    nodes.find((node) => node.id === "workstation:router")?.data.handles ?? [],
  );
  const modelWorkstationHandles = handleIds(
    nodes.find((node) => node.id === "workstation:draft")?.data.handles ?? [],
  );

  expect(logicalMoveHandles).not.toContain("worker-assignment-target");
  expect(modelWorkstationHandles).toContain("worker-assignment-target");
});
