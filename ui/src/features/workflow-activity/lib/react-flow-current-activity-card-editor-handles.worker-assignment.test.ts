import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft.test-helpers";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/factory-validation-graph-projection";
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

const standardProcessorWithoutStopWords = factoryGraphConnectionAnchorContext({
  type: "MODEL_WORKSTATION",
  behavior: "STANDARD",
});

const standardProcessorWithStopWords = factoryGraphConnectionAnchorContext({
  type: "MODEL_WORKSTATION",
  behavior: "STANDARD",
  stopWords: ["DONE"],
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

it("omits continue, failure, and reject handles on LOGICAL_MOVE workstations in connect mode", () => {
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

  expect(ids).not.toContain("workstation-on-continue-source");
  expect(ids).not.toContain("workstation-on-failure-source");
  expect(ids).not.toContain("workstation-on-rejection-source");
});

it("keeps failure and omits continue and reject for a standard processor without stopWords", () => {
  const ids = handleIds(
    buildEditorHandles({
      connectionAnchorContext: standardProcessorWithoutStopWords,
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

  expect(ids).toContain("workstation-on-failure-source");
  expect(ids).not.toContain("workstation-on-continue-source");
  expect(ids).not.toContain("workstation-on-rejection-source");
});

it("renders all progress-outcome handles when stopWords are configured", () => {
  const ids = handleIds(
    buildEditorHandles({
      connectionAnchorContext: standardProcessorWithStopWords,
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

  expect(ids).toEqual(
    expect.arrayContaining([
      "workstation-on-continue-source",
      "workstation-on-failure-source",
      "workstation-on-rejection-source",
    ]),
  );
});

it("does not attach progress-outcome validation to hidden logical-move anchors", () => {
  const validationProjection = projectFactoryValidationTargets([
    {
      code: "factory.workstation.missingFailureRoute",
      message: "Workstation router must define a failure route.",
      severity: "error",
      subject: {
        id: "router",
        location: "ON_FAILURE",
        type: "WORKSTATION",
      },
    },
    {
      code: "factory.workstation.missingRejectionRoute",
      message: "Workstation router must define a reject route.",
      severity: "error",
      subject: {
        id: "router",
        location: "ON_REJECTION",
        type: "WORKSTATION",
      },
    },
    {
      code: "factory.workstation.missingContinueRoute",
      message: "Workstation router must define a continue route.",
      severity: "error",
      subject: {
        id: "router",
        location: "ON_CONTINUE",
        type: "WORKSTATION",
      },
    },
  ]);
  const handles = buildEditorHandles({
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
    validationProjection,
  });

  expect(handles.find((handle) => handle.id === "workstation-on-failure-source")).toBeUndefined();
  expect(handles.find((handle) => handle.id === "workstation-on-rejection-source")).toBeUndefined();
  expect(handles.find((handle) => handle.id === "workstation-on-continue-source")).toBeUndefined();
});

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

it("resolves assigned worker stop tokens from factory definitions", () => {
  const factoryWithWorkerStopToken = {
    ...baseFactoryDefinition,
    workers: [
      {
        name: "processor",
        stopToken: "<COMPLETE>",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      ...(baseFactoryDefinition.workstations ?? []),
      {
        body: "Review the story.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        worker: "processor",
      },
    ],
  } satisfies CanonicalFactoryDefinition;

  expect(
    resolveWorkstationConnectionAnchorContext(
      factoryWithWorkerStopToken,
      "workstation:review",
    )?.workstation,
  ).toEqual(
    expect.objectContaining({
      assignedWorkerStopToken: "<COMPLETE>",
      type: "MODEL_WORKSTATION",
    }),
  );
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
    onSelectResource: () => {},
    onSelectWorker: () => {},
    onSelectWorkType: () => {},
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
  expect(logicalMoveHandles).not.toContain("workstation-on-continue-source");
  expect(logicalMoveHandles).not.toContain("workstation-on-failure-source");
  expect(logicalMoveHandles).not.toContain("workstation-on-rejection-source");
  expect(modelWorkstationHandles).toContain("worker-assignment-target");
});
