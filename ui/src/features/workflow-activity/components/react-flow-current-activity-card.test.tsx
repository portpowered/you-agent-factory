import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactElement } from "react";
import type {
  DashboardPlaceRef,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import {
  type FactoryValue,
  NamedFactoryAPIError,
} from "../../../api/named-factory";
import { factoryFromDashboardTopology } from "../../../components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  resourceOccupancySnapshotForTick,
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
  workstationKindParityDashboardSnapshot,
  workstationKindParityExpectations,
} from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinition,
  baseFactoryDefinitionDocument,
  createMockGraphEditorDraftState,
  wireMockEditableFactoryGraph,
  workerDenseFactoryDefinitionDocument,
} from "../../../testing/graph-editor-harness";
import {
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} from "../../current-factory-definition/public";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "../../factory-graph-editor/lib/factory-graph-customer-display";
import { removeFactoryGraphNode } from "../../factory-graph-editor/lib/factory-graph-operations";
import { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import { useEditableFactoryGraph } from "../../factory-graph-editor/hooks/use-editable-factory-graph";
import {
  EXHAUSTION_WORKSTATION_ICON_METADATA,
  SUPPORTED_WORKSTATION_ICON_METADATA,
} from "../../flowchart/lib/workstation-icon-metadata";
import type { ReadFactoryImportFile } from "../../import/hooks/use-factory-png-drop";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import { getImportPreviewDialogMessages } from "../../import/messages/import-preview-dialog";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "../lib/current-activity-factory-graph-layout";
import { getDashboardFlowAxisLegendMessages } from "../messages/dashboard-flow-axis-legend";
import { getWorkflowActivityGraphImportMessages } from "../messages/graph-import";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import {
  currentActivityGraphKey,
  currentActivityTopologyKey,
  ReactFlowCurrentActivityCard,
} from "./react-flow-current-activity-card";

vi.mock("../../current-factory-definition/public", async () => {
  const actual = await vi.importActual(
    "../../current-factory-definition/public",
  );

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
    useSaveCurrentFactory: vi.fn(),
  };
});

vi.mock("../../factory-graph-editor/hooks/use-editable-factory-graph", async () => {
  const actual = await vi.importActual(
    "../../factory-graph-editor/hooks/use-editable-factory-graph",
  );

  return {
    ...actual,
    useEditableFactoryGraph: vi.fn(),
  };
});

vi.mock("../../factory-graph-editor/hooks/factory-graph-draft-hook", async () => {
  const actual = await vi.importActual(
    "../../factory-graph-editor/hooks/factory-graph-draft-hook",
  );

  return {
    ...actual,
    useFactoryGraphDraftState: vi.fn(),
  };
});

const PADDING_CLASS_PATTERN = /(^|\s)p[trblxy]?-[^\s]+/;

interface RenderCurrentActivityOptions {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  importController?: CurrentActivityImportController;
  locale?: string;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  snapshot: DashboardSnapshot;
  selection?: CurrentActivitySelection | null;
  widgetInstanceID?: string;
}

const LEGEND_ICON_EXPECTATIONS = [
  ["Queue", "queue"],
  ["Processing", "processing"],
  ["Terminal", "terminal"],
  ["Failed state", "failed"],
  ["Resource", "resource"],
  ["Constraint", "constraint"],
  ["Limit", "limit"],
  ...SUPPORTED_WORKSTATION_ICON_METADATA.map((metadata) => [
    metadata.label,
    metadata.iconKind,
  ]),
  ["Active work", "active-work"],
  [
    EXHAUSTION_WORKSTATION_ICON_METADATA.label,
    EXHAUSTION_WORKSTATION_ICON_METADATA.iconKind,
  ],
] as const;
const workflowGraphLocaleFallbackTimeoutMs = 180_000;

const defaultDraftState = createMockGraphEditorDraftState();

function dashboardSnapshotWithStateCounts(
  overrides: Record<string, number>,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.runtime.place_token_counts = {
    ...snapshot.runtime.place_token_counts,
    ...overrides,
  };

  return snapshot;
}

function refreshFactoryFromTopology(
  snapshot: DashboardSnapshot,
): DashboardSnapshot {
  snapshot.factory = factoryFromDashboardTopology(snapshot.topology);
  return snapshot;
}

function dashboardSnapshotWithEditableFactory(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.factory = structuredClone(baseFactoryDefinition);
  return snapshot;
}

function workerDenseSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.runtime.active_executions_by_dispatch_id = {
    "dispatch-draft-active": {
      dispatch_id: "dispatch-draft-active",
      started_at: "2026-05-19T01:10:00Z",
      transition_id: "draft-transition",
      workstation_name: "draft",
      workstation_node_id: "draft",
    },
  };
  snapshot.runtime.active_dispatch_ids = ["dispatch-draft-active"];
  snapshot.runtime.active_workstation_node_ids = ["draft"];
  snapshot.runtime.active_throttle_pauses = [
    {
      affected_worker_types: ["stalled"],
      lane_id: "provider:codex",
      model: "gpt-5",
      paused_until: "2026-05-19T01:15:00Z",
      provider: "OPENAI",
      recover_at: "2026-05-19T01:16:00Z",
    },
  ];
  snapshot.runtime.workstation_requests_by_dispatch_id = {
    "dispatch-review": {
      counts: {
        dispatched_count: 1,
        errored_count: 1,
        responded_count: 1,
      },
      dispatch_id: "dispatch-review",
      request: {},
      response: {
        failure_message: "Provider request failed.",
        outcome: "FAILED",
      },
      transition_id: "review-transition",
      workstation_name: "review",
      workstation_node_id: "review",
      work_items: [],
    },
  };

  return snapshot;
}

function canonicalObserverSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.factory = {
    name: "canonical-observer",
    resources: [{ capacity: 2, name: "agent-slot" }],
    workers: [
      {
        model: "gpt-5",
        name: "planner",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5",
        name: "agent",
        resources: [{ capacity: 1, name: "agent-slot" }],
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "init", type: "INITIAL" },
          { name: "ready", type: "PROCESSING" },
          { name: "implemented", type: "PROCESSING" },
          { name: "documented", type: "PROCESSING" },
          { name: "blocked", type: "FAILED" },
          { name: "complete", type: "TERMINAL" },
        ],
      },
    ],
    workstations: [
      {
        behavior: "STANDARD",
        id: "plan",
        inputs: [{ state: "init", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "ready", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "planner",
      },
      {
        behavior: "REPEATER",
        id: "implement",
        inputs: [{ state: "ready", workType: "story" }],
        name: "Implement",
        onFailure: [{ state: "blocked", workType: "story" }],
        outputs: [{ state: "implemented", workType: "story" }],
        resources: [{ capacity: 1, name: "agent-slot" }],
        type: "MODEL_WORKSTATION",
        worker: "agent",
      },
      {
        behavior: "STANDARD",
        id: "document",
        inputs: [{ state: "ready", workType: "story" }],
        name: "Document",
        outputs: [{ state: "documented", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "agent",
      },
      {
        behavior: "REPEATER",
        id: "review",
        inputs: [
          { state: "implemented", workType: "story" },
          { state: "documented", workType: "story" },
        ],
        name: "Review",
        onContinue: [{ state: "ready", workType: "story" }],
        outputs: [{ state: "complete", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
      {
        behavior: "STANDARD",
        id: "repair",
        inputs: [{ state: "blocked", workType: "story" }],
        name: "Repair",
        outputs: [{ state: "ready", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "agent",
      },
    ],
  };
  snapshot.topology = {
    edges: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  };

  return snapshot;
}

async function getStateNodeArticle(label: string): Promise<HTMLElement> {
  const button = await screen.findByRole("button", {
    name: `Select ${label} state`,
  });
  return button.closest(".react-flow__node") as HTMLElement;
}

async function getWorkstationNode(label = "Review"): Promise<HTMLElement> {
  const button = await screen.findByRole("button", {
    name: `Select ${label} workstation`,
  });
  return button.closest(".react-flow__node") as HTMLElement;
}

function expectFixedWorkstationNodeDimensions(node: Element | null) {
  expect(node?.getAttribute("style")).toContain("width: 156px");
  expect(node?.getAttribute("style")).toContain("height: 196px");
}

function renderCurrentActivity({
  activateFactory,
  importController,
  locale,
  onFactoryActivated,
  onFactoryImportReady,
  readFactoryImportFile,
  snapshot,
  selection = null,
  widgetInstanceID,
}: RenderCurrentActivityOptions) {
  const onSelectWorkID =
    vi.fn<
      (workID: string, hint?: { dispatchID?: string; nodeID?: string }) => void
    >();
  const onSelectStateNode = vi.fn<(placeId: string) => void>();
  const onSelectWorker = vi.fn<(workerName: string) => void>();
  const onSelectWorkstation = vi.fn<(nodeId: string) => void>();

  renderWithQueryClient(
    <ReactFlowCurrentActivityCard
      activateFactory={activateFactory}
      importController={importController}
      locale={locale}
      now={Date.parse("2026-04-08T12:00:04Z")}
      onFactoryActivated={onFactoryActivated}
      onFactoryImportReady={onFactoryImportReady}
      onSelectWorkID={onSelectWorkID}
      onSelectStateNode={onSelectStateNode}
      onSelectWorker={onSelectWorker}
      onSelectWorkstation={onSelectWorkstation}
      readFactoryImportFile={readFactoryImportFile}
      selection={selection}
      snapshot={snapshot}
      widgetInstanceID={widgetInstanceID}
    />,
  );

  return {
    onSelectStateNode,
    onSelectWorkID,
    onSelectWorker,
    onSelectWorkstation,
  };
}

function renderWithQueryClient(view: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}

function createFactoryImportValue(): FactoryPngImportValue {
  return {
    factory: {
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: vi.fn(),
    schemaVersion: "portos.agent-factory.png.v1",
  };
}

function createImportController(
  overrides: Partial<CurrentActivityImportController> = {},
): CurrentActivityImportController {
  return {
    activateImport: vi.fn().mockResolvedValue(undefined),
    activationState: { status: "idle" },
    clearActivationError: vi.fn(),
    clearError: vi.fn(),
    closeImportPreview: vi.fn(),
    dropState: { status: "idle" },
    importPreviewState: { status: "idle" },
    onDragEnter: vi.fn(),
    onDragLeave: vi.fn(),
    onDragOver: vi.fn(),
    onDrop: vi.fn(),
    ...overrides,
  };
}

function createFileDropTransfer(files: File[]): {
  dataTransfer: {
    dropEffect: string;
    files: File[];
    types: string[];
  };
} {
  return {
    dataTransfer: {
      dropEffect: "none",
      files,
      types: ["Files"],
    },
  };
}

async function expandGraphLegend(locale = "en"): Promise<HTMLElement> {
  const messages = getDashboardFlowAxisLegendMessages(locale);
  const actionTargetLabel =
    messages.title.charAt(0).toLowerCase() + messages.title.slice(1);
  const expandButton = await screen.findByRole("button", {
    name: messages.expandToggleLabel(actionTargetLabel),
  });

  fireEvent.click(expandButton);

  return await screen.findByLabelText(messages.title);
}

function dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(): DashboardSnapshot {
  return dashboardSnapshotWithActiveWorkLabels(
    [
      "Short Active Story",
      "Active Story With A Medium Sized Label",
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    ],
    "Review Requests With A Deliberately Long Workstation Title",
  );
}

function dashboardSnapshotWithActiveWorkLabels(
  labels: string[],
  workstationName?: string,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[
      "dispatch-review-active"
    ];
  const reviewWorkstation = snapshot.topology.workstation_nodes_by_id.review;

  if (reviewWorkstation && workstationName) {
    reviewWorkstation.workstation_name = workstationName;
  }

  if (activeExecution) {
    activeExecution.work_items = labels.map(
      (label, index): DashboardWorkItemRef => {
        const itemNumber = index + 1;

        return {
          display_name: label,
          trace_id: `trace-active-story-${itemNumber}`,
          work_id: `work-active-story-${itemNumber}`,
          work_type_id: "story",
        };
      },
    );
    activeExecution.trace_ids = activeExecution.work_items.map(
      (workItem) => workItem.trace_id ?? workItem.work_id,
    );
  }

  return refreshFactoryFromTopology(snapshot);
}

function dashboardSnapshotWithLongStateLabels(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  for (const workstation of Object.values(
    snapshot.topology.workstation_nodes_by_id,
  )) {
    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      if (place.place_id === "story:ready") {
        place.type_id =
          "customer-escalation-story-with-a-deliberately-long-type";
        place.state_value =
          "ready-for-review-after-multiple-dependent-checks-complete";
      }
    }
  }

  return refreshFactoryFromTopology(snapshot);
}

function dashboardSnapshotWithActiveWorkItemCount(
  count: number,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const reviewActivity =
    snapshot.runtime.workstation_activity_by_node_id?.review;
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[
      "dispatch-review-active"
    ];
  const workItems = Array.from(
    { length: count },
    (_, index): DashboardWorkItemRef => {
      const itemNumber = index + 1;

      return {
        display_name: `Active Story ${itemNumber}`,
        trace_id: `trace-active-story-${itemNumber}`,
        work_id: `work-active-story-${itemNumber}`,
        work_type_id: "story",
      };
    },
  );

  if (count === 0) {
    snapshot.runtime.active_dispatch_ids = (
      snapshot.runtime.active_dispatch_ids ?? []
    ).filter((dispatchID) => dispatchID !== "dispatch-review-active");
    snapshot.runtime.active_workstation_node_ids = (
      snapshot.runtime.active_workstation_node_ids ?? []
    ).filter((nodeID) => nodeID !== "review");
    if (snapshot.runtime.active_executions_by_dispatch_id) {
      delete snapshot.runtime.active_executions_by_dispatch_id[
        "dispatch-review-active"
      ];
    }
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = [];
      reviewActivity.active_work_items = [];
      reviewActivity.trace_ids = [];
    }
  } else if (activeExecution) {
    activeExecution.work_items = workItems;
    activeExecution.trace_ids = workItems.map(
      (workItem) => workItem.trace_id ?? workItem.work_id,
    );
    snapshot.runtime.active_dispatch_ids = ["dispatch-review-active"];
    snapshot.runtime.active_workstation_node_ids = ["review"];
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = ["dispatch-review-active"];
      reviewActivity.active_work_items = workItems;
      reviewActivity.trace_ids = workItems.map(
        (workItem) => workItem.trace_id ?? workItem.work_id,
      );
    }
  }

  snapshot.runtime.in_flight_dispatch_count = count;

  return refreshFactoryFromTopology(snapshot);
}

function dashboardSnapshotWithActiveImplementWorkstation(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[
      "dispatch-review-active"
    ];
  const implementWorkstation =
    snapshot.topology.workstation_nodes_by_id.implement;

  if (activeExecution && implementWorkstation) {
    activeExecution.workstation_node_id = "implement";
    activeExecution.transition_id = implementWorkstation.transition_id;
    activeExecution.workstation_name = implementWorkstation.workstation_name;
    activeExecution.consumed_tokens = [
      {
        token_id: "token-implement-story",
        place_id: "story:ready",
        name: "Active Story",
        work_id: "work-active-story",
        work_type_id: "story",
        trace_id: "trace-active-story",
        created_at: "2026-04-08T12:00:00Z",
        entered_at: "2026-04-08T12:00:00Z",
      },
      {
        token_id: "token-implement-agent-slot",
        place_id: "agent-slot:available",
        name: "Agent Slot",
        work_id: "resource-agent-slot",
        work_type_id: "agent-slot",
        created_at: "2026-04-08T12:00:00Z",
        entered_at: "2026-04-08T12:00:00Z",
      },
    ];
  }

  snapshot.runtime.active_workstation_node_ids = ["implement"];
  snapshot.runtime.current_work_items_by_place_id = {
    ...(snapshot.runtime.current_work_items_by_place_id ?? {}),
    "story:ready": [
      {
        display_name: "Active Story",
        trace_id: "trace-active-story",
        work_id: "work-active-story",
        work_type_id: "story",
      },
    ],
  };

  return refreshFactoryFromTopology(snapshot);
}

function dashboardSnapshotWithResourceReturnEdge(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const implementWorkstation =
    snapshot.topology.workstation_nodes_by_id.implement;
  const agentSlotPlace: DashboardPlaceRef = {
    kind: "resource",
    place_id: "agent-slot:available",
    state_value: "available",
    type_id: "agent-slot",
  };

  if (implementWorkstation) {
    implementWorkstation.output_places = [
      ...(implementWorkstation.output_places ?? []),
      agentSlotPlace,
    ];
    implementWorkstation.output_place_ids = [
      ...(implementWorkstation.output_place_ids ?? []),
      agentSlotPlace.place_id,
    ];
  }

  return refreshFactoryFromTopology(snapshot);
}

function cronSystemTimeFactoryDefinition(): CanonicalFactoryDefinition {
  return {
    name: "cron-system-time-card",
    workers: [
      {
        command: "./cron.sh",
        name: "scheduler",
        type: "SCRIPT_WORKER",
      },
      {
        model: "gpt-5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "scheduled", type: "PROCESSING" },
          { name: "done", type: "TERMINAL" },
        ],
      },
      {
        name: "schedule",
        states: [{ name: "tick", type: "INITIAL" }],
      },
      {
        name: SYSTEM_TIME_WORK_TYPE_ID,
        states: [{ name: "pending", type: "PROCESSING" }],
      },
    ],
    workstations: [
      {
        behavior: "CRON",
        cron: { schedule: "0 0 * * *" },
        id: "nightly-cron",
        inputs: [{ state: "tick", workType: "schedule" }],
        name: "Nightly Cron",
        outputs: [{ state: "scheduled", workType: "story" }],
        worker: "scheduler",
      },
      {
        behavior: "STANDARD",
        id: "review",
        inputs: [{ state: "scheduled", workType: "story" }],
        name: "Review",
        outputs: [{ state: "done", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
      {
        id: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
        inputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
        name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
        outputs: [],
        worker: "",
      },
    ],
  } satisfies CanonicalFactoryDefinition;
}

function dashboardSnapshotWithCronSystemTimeFactory(): DashboardSnapshot {
  const factory = cronSystemTimeFactoryDefinition();
  const workstations = (factory.workstations ?? []).map(
    dashboardWorkstationFromFactory,
  );

  return {
    factory,
    factory_state: "IDLE",
    runtime: {
      active_executions_by_dispatch_id: {},
      current_work_items_by_place_id: {},
      place_occupancy_work_items_by_place_id: {},
      place_token_counts: {
        "schedule:tick": 1,
        "story:scheduled": 1,
      },
      session: {
        completed_count: 0,
        dispatched_count: 0,
        failed_count: 0,
        has_data: true,
        provider_sessions: [],
      },
      workstation_requests_by_dispatch_id: {},
    },
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
  };
}

function dashboardSnapshotWithExhaustionRuleNode(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  snapshot.topology.workstation_node_ids = [
    ...snapshot.topology.workstation_node_ids,
    "executor-loop-breaker",
  ];
  snapshot.topology.workstation_nodes_by_id["executor-loop-breaker"] = {
    input_place_ids: ["story:ready"],
    input_places: [
      {
        kind: "work_state",
        place_id: "story:ready",
        state_category: "PROCESSING",
        state_value: "ready",
        type_id: "story",
      },
    ],
    node_id: "executor-loop-breaker",
    output_place_ids: ["story:blocked"],
    output_places: [
      {
        kind: "work_state",
        place_id: "story:blocked",
        state_category: "FAILED",
        state_value: "blocked",
        type_id: "story",
      },
    ],
    transition_id: "executor-loop-breaker",
    workstation_name: "executor-loop-breaker",
  };
  snapshot.runtime.active_dispatch_ids = [
    ...(snapshot.runtime.active_dispatch_ids ?? []),
    "dispatch-exhaustion-should-not-render-work",
  ];
  snapshot.runtime.active_executions_by_dispatch_id = {
    ...(snapshot.runtime.active_executions_by_dispatch_id ?? {}),
    "dispatch-exhaustion-should-not-render-work": {
      consumed_tokens: [],
      dispatch_id: "dispatch-exhaustion-should-not-render-work",
      started_at: "2026-04-08T12:00:00Z",
      transition_id: "executor-loop-breaker",
      workstation_node_id: "executor-loop-breaker",
      workstation_name: "executor-loop-breaker",
      work_items: [
        {
          display_name: "Should Not Render",
          trace_id: "trace-hidden-exhaustion",
          work_id: "work-hidden-exhaustion",
          work_type_id: "story",
        },
      ],
    },
  };

  return snapshot;
}

let restoreBrowserTestShims: (() => void) | null = null;

function registerCurrentActivityCardTestLifecycle(): void {
  beforeEach(() => {
    window.localStorage.clear();
    useCurrentActivityGraphStore.setState({ positionsByGraphKey: {} });
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);
    vi.mocked(useSaveCurrentFactory).mockReturnValue({
      mutateAsync: vi.fn(),
      reset: vi.fn(),
      status: "idle",
    } as never);
    wireMockEditableFactoryGraph(
      {
        useEditableFactoryGraph: vi.mocked(useEditableFactoryGraph),
        useFactoryGraphDraftState: vi.mocked(useFactoryGraphDraftState),
      },
      defaultDraftState,
    );
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.clearAllMocks();
  });

  it("keeps editor controls unavailable until the graph editor mode is enabled", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    expect(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("region", { name: "Factory graph editor tools" }),
    ).toBeNull();
    expect(screen.getByText("Observe mode")).toBeTruthy();
  });

  it("shows the add, delete, and connect toolbar in editor mode", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(toolbar).getByRole("button", { name: "Open add entity menu" }),
    ).toBeTruthy();
    expect(
      within(toolbar).getByRole("button", { name: "Delete" }),
    ).toBeTruthy();
    expect(
      within(toolbar).getByRole("button", { name: "Connect" }),
    ).toBeTruthy();
    expect(screen.getByText("Editor mode active")).toBeTruthy();
  });

  it("keeps classifier workstations out of the editable graph flow", async () => {
    const snapshot = dashboardSnapshotWithActiveWorkItemCount(0);
    snapshot.topology.workstation_nodes_by_id.review.workstation_kind =
      "CLASSIFIER_WORKSTATION";
    snapshot.factory = structuredClone(baseFactoryDefinition);
    const reviewWorkstation = snapshot.factory?.workstations?.find(
      (workstation) => workstation.name === "review",
    );
    if (reviewWorkstation) {
      reviewWorkstation.type = "CLASSIFIER_WORKSTATION";
      reviewWorkstation.classificationRoutes = [
        {
          label: "approved",
          output: {
            state: "complete",
            workType: "story",
          },
        },
      ];
    }

    renderCurrentActivity({
      snapshot,
    });

    const enterEditorButton = screen.getByRole("button", {
      name: 'Factory graph editing does not yet support classifier workstation routes. "review" stays read-only in this view until labeled route editing is available.',
    });
    expect(enterEditorButton.getAttribute("disabled")).not.toBeNull();
    expect(
      screen.getByText(
        'Editor unavailable: Factory graph editing does not yet support classifier workstation routes. "review" stays read-only in this view until labeled route editing is available.',
      ),
    ).toBeTruthy();

    fireEvent.click(enterEditorButton);

    await waitFor(() => {
      expect(
        screen.queryByRole("region", { name: "Factory graph editor tools" }),
      ).toBeNull();
    });
    expect(screen.queryByText("Observe mode")).toBeNull();
  });

  it("lists the supported add-entity options and validates duplicate worker names before mutating the draft", async () => {
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Open add entity menu" }),
    );

    expect(screen.getByRole("button", { name: "Workstation" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Worker" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Work type" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Work state" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Resource" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Worker" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "writer" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(
      screen.getByText('A worker named "writer" already exists in the draft.'),
    ).toBeTruthy();
    expect(replaceDraft).not.toHaveBeenCalled();
  });

  it("submits valid add-entity forms into the pending graph draft", async () => {
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Open add entity menu" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Work type" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "essay" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "First state" }), {
      target: { value: "queued" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(replaceDraft).toHaveBeenCalledTimes(1);
    const nextDraft = replaceDraft.mock.calls[0]?.[0];
    expect(nextDraft.additions.workTypes).toEqual([
      {
        name: "essay",
        states: [
          {
            name: "queued",
            type: "INITIAL",
          },
        ],
      },
    ]);
  });

  it("distinguishes work-state creation from work-type creation and blocks missing work-type association", async () => {
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Open add entity menu" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Work state" }));

    expect(screen.getByText("Add work state")).toBeTruthy();
    expect(
      screen.getByText("Append a new ordered state to an existing work type."),
    ).toBeTruthy();
    expect(
      (
        screen.getByRole("combobox", {
          name: "Work type",
        }) as HTMLSelectElement
      ).value,
    ).toBe("story");
    expect(
      (
        screen.getByRole("combobox", {
          name: "State type",
        }) as HTMLSelectElement
      ).value,
    ).toBe("PROCESSING");

    fireEvent.change(screen.getByRole("combobox", { name: "Work type" }), {
      target: { value: "" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(
      screen.getByText("Choose a work type before adding a work state."),
    ).toBeTruthy();
    expect(replaceDraft).not.toHaveBeenCalled();
  });

  it("submits valid work-state add-entity forms into the pending graph draft", async () => {
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithEditableFactory(),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Open add entity menu" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Work state" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "approved" },
    });
    fireEvent.change(screen.getByRole("combobox", { name: "State type" }), {
      target: { value: "TERMINAL" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(replaceDraft).toHaveBeenCalledTimes(1);
    const nextDraft = replaceDraft.mock.calls[0]?.[0];

    expect(nextDraft.additions.workStates).toEqual([
      {
        state: {
          name: "approved",
          type: "TERMINAL",
        },
        workTypeName: "story",
      },
    ]);
  }, 30_000);

  it("keeps editor mode on the shared observer graph surface", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      graph: {
        edges: [],
        nodes: [
          {
            id: "worker:writer",
            key: { kind: "worker", name: "writer" },
            kind: "worker",
            label: "writer",
          },
          {
            id: "workstation:review",
            key: { kind: "workstation", name: "review" },
            kind: "workstation",
            label: "review",
          },
        ],
      },
      hasChanges: true,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workstations: [
            {
              inputs: [],
              name: "review",
              outputs: [],
              type: "MODEL_WORKSTATION",
              worker: "writer",
            },
          ],
        },
      },
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    await waitFor(() => {
      expect(
        document.querySelector(
          '[data-current-activity-node-type="workstation"]',
        ),
      ).toBeTruthy();
    });
    expect(screen.queryByText("Pending")).toBeNull();
  });

  it("renders worker and resource nodes from the canonical snapshot factory in observer mode", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: workerDenseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: workerDenseFactoryDefinitionDocument,
      pendingFactoryDefinition: workerDenseFactoryDefinitionDocument,
    } as never);
    const snapshot = workerDenseSnapshot();
    snapshot.factory = workerDenseFactoryDefinitionDocument;

    renderCurrentActivity({
      snapshot,
    });

    await waitFor(() => {
      expect(
        document.querySelector(
          '[data-current-activity-node-type="workstation"]',
        ),
      ).toBeTruthy();
    });

    await waitFor(() => {
      expect(document.querySelector('[data-id="worker:writer"]')).toBeTruthy();
      expect(
        document.querySelector('[data-id="worker:reviewer"]'),
      ).toBeTruthy();
      expect(document.querySelector('[data-id="worker:stalled"]')).toBeTruthy();
      expect(document.querySelector('[data-id="resource:gpu"]')).toBeTruthy();
    });
  });

  it("does not render the editor-only visibility preset controls in embedded editor mode", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: workerDenseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: workerDenseFactoryDefinitionDocument,
      pendingFactoryDefinition: workerDenseFactoryDefinitionDocument,
    } as never);

    renderCurrentActivity({
      snapshot: workerDenseSnapshot(),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    await waitFor(() => {
      expect(
        document.querySelector(
          '[data-current-activity-node-type="workstation"]',
        ),
      ).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Infrastructure" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Workflow" })).toBeNull();
    expect(screen.queryByRole("button", { name: "All" })).toBeNull();
  });

  it("renders supported workstation and work-state editor handles on the shared observer graph", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    expect(
      await screen.findAllByRole("button", {
        name: "Route successful output from this workstation.",
      }),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByRole("button", {
        name: "Accept an input work state for this workstation.",
      }),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByRole("button", {
        name: "Route this work state into a workstation input.",
      }),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByRole("button", {
        name: "Receive workstation output into this work state.",
      }),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByRole("button", {
        name: "Assign this worker to a workstation.",
      }),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByRole("button", {
        name: "Accept a worker assignment for this workstation.",
      }),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByRole("button", {
        name: "Accept a resource requirement for this workstation.",
      }),
    ).not.toHaveLength(0);
    expect(
      screen.getAllByRole("button", {
        name: "Provide this resource to a workstation.",
      }),
    ).not.toHaveLength(0);
  });

  it("removes a workstation without opening a confirmation", () => {
    const result = removeFactoryGraphNode({
      baseFactoryDefinition: baseFactoryDefinitionDocument,
      draft: defaultDraftState.draft,
      nodeId: "workstation:review",
    });

    expect(result.ok).toBe(true);
    if (!result.ok) {
      return;
    }
    expect(result.value.removals.workstations).toEqual(["review"]);
  });

  it("keeps worker nodes visible but not workstation-style deletion targets", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithEditableFactory(),
    });

    expect(
      await screen.findByRole("button", {
        name: "Select writer worker",
      }),
    ).toBeTruthy();
    expect(await screen.findByLabelText("worker:writer")).toBeTruthy();
    expect(
      await screen.findByRole("button", {
        name: /Select .* workstation/,
      }),
    ).toBeTruthy();

    const removeWorker = removeFactoryGraphNode({
      baseFactoryDefinition: baseFactoryDefinitionDocument,
      draft: defaultDraftState.draft,
      nodeId: "worker:writer",
    });
    expect(removeWorker).toMatchObject({
      ok: false,
      reason: "BLOCKED_REMOVAL",
    });
  });

  it("keeps removed server-backed workstations visible with a pending-removal badge", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        removals: {
          ...defaultDraftState.draft.removals,
          workstations: ["review"],
        },
      },
      graph: {
        edges: [],
        nodes: [],
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    expect(
      await screen.findByRole("button", {
        name: "Select Review workstation",
      }),
    ).toBeTruthy();
    expect(screen.getByText("Unsaved graph changes")).toBeTruthy();
  });

  it("shows a loading editor state while the editable definition is still fetching", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(screen.getByText("Loading editor definition")).toBeTruthy();
    expect(
      within(toolbar)
        .getByRole("button", { name: "Open add entity menu" })
        .getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      within(toolbar)
        .getByRole("button", { name: "Delete" })
        .getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      within(toolbar)
        .getByRole("button", { name: "Connect" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("requires save, discard, or keep editing before leaving editor mode with unsaved changes", async () => {
    const resetDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      hasChanges: true,
      resetDraft,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Leave factory graph editor" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Leave graph editor with unsaved changes?",
    });
    expect(
      within(dialog).getByRole("button", { name: "Save changes" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "Discard changes" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "Keep editing" }),
    ).toBeTruthy();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Discard changes" }),
    );

    expect(resetDraft).toHaveBeenCalledTimes(1);
  });

  it("shows explicit save and discard actions for pending graph changes", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    const actions = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(actions).getByRole("button", { name: "Discard changes" }),
    ).toBeTruthy();
    expect(
      within(actions).getByRole("button", { name: "Save changes" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("region", { name: "Pending graph changes" }),
    ).toBeNull();
  });

  it("confirms pending save changes before saving the graph draft", async () => {
    const mutateAsync = vi
      .fn()
      .mockResolvedValue(baseFactoryDefinitionDocument);
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useSaveCurrentFactory).mockReturnValue({
      mutateAsync,
      reset: vi.fn(),
      status: "idle",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    fireEvent.click(
      within(
        await screen.findByRole("region", {
          name: "Factory graph editor tools",
        }),
      ).getByRole("button", { name: "Save changes" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Save factory graph changes?",
    });
    expect(
      within(dialog).getByText("This save will apply 1 created entity."),
    ).toBeTruthy();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Save topology" }),
    );

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        baseVersion: baseFactoryDefinitionDocument.version,
        factoryDefinition: baseFactoryDefinition,
      });
    });
  });

  it("warns when a newer editable-definition version arrives during a dirty draft", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      baseDocument: baseFactoryDefinitionDocument,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
      latestDocument: {
        ...baseFactoryDefinitionDocument,
        version: {
          logical: "9",
          physical: "2026-05-19T01:45:00Z",
        },
      },
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    expect(
      await screen.findByText("A newer factory definition is available"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Refresh or discard the current draft before saving so you do not overwrite a newer topology version.",
      ),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("blocks topology save while active work is still running", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(1),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );

    expect(await screen.findByText("Topology edits are blocked")).toBeTruthy();
    expect(
      screen.getByText(
        "Save is unavailable while active work is still running in the current factory.",
      ),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("saves the pending editable definition before leaving editor mode", async () => {
    const mutateAsync = vi
      .fn()
      .mockResolvedValue(baseFactoryDefinitionDocument);
    const replaceDraft = vi.fn();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useSaveCurrentFactory).mockReturnValue({
      mutateAsync,
      reset: vi.fn(),
      status: "idle",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      hasChanges: true,
      replaceDraft,
    } as never);

    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(0),
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Leave factory graph editor" }),
    );
    fireEvent.click(
      within(
        await screen.findByRole("dialog", {
          name: "Leave graph editor with unsaved changes?",
        }),
      ).getByRole("button", { name: "Save changes" }),
    );

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        baseVersion: baseFactoryDefinitionDocument.version,
        factoryDefinition: baseFactoryDefinition,
      });
    });
    expect(replaceDraft).toHaveBeenCalledTimes(1);
  });
}

describe("ReactFlowCurrentActivityCard import flows", () => {
  registerCurrentActivityCardTestLifecycle();

  it("scopes file drag-over and drop handling to the graph viewport and opens a preview", async () => {
    const legendMessages = getDashboardFlowAxisLegendMessages("en");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });
    const legendToggle = screen.getByRole("button", {
      name: legendMessages.expandToggleLabel("graph legend"),
    });

    fireEvent.dragOver(legendToggle, createFileDropTransfer([file]));

    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );
    expect(readFactoryImportFile).not.toHaveBeenCalled();

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "drag-active",
    );
    expect(screen.getByText("Import factory PNG")).toBeTruthy();
    expect(
      screen.getByText(
        "Drop a you-agent-factory PNG onto this graph to start import.",
      ),
    ).toBeTruthy();

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    await waitFor(() => {
      expect(readFactoryImportFile).toHaveBeenCalledWith(file);
    });
    await waitFor(() => {
      expect(onFactoryImportReady).toHaveBeenCalledWith(importValue, file);
    });
    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");
    expect(previewDialog.textContent).toContain(
      "Review the dropped factory before activation.",
    );
    expect(
      within(previewDialog)
        .getByRole("img", { name: "Dropped Factory preview" })
        .getAttribute("src"),
    ).toBe("blob:factory-preview");
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Cancel import" }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Review factory import" }),
      ).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
  });

  it("closes the factory import preview from the shared dialog close control", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });
    const closeButton = within(previewDialog).getByRole("button", {
      name: "Close import preview",
    });

    fireEvent.click(closeButton);

    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Review factory import" }),
      ).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(onFactoryImportReady).toHaveBeenCalledWith(importValue, file);
  });

  it("does not render the import preview inside the graph card when a dashboard controller owns it", () => {
    renderCurrentActivity({
      importController: createImportController({
        importPreviewState: {
          file: new File(["png"], "factory-import.png", { type: "image/png" }),
          status: "ready",
          value: createFactoryImportValue(),
        },
      }),
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    expect(
      screen.queryByRole("dialog", { name: "Review factory import" }),
    ).toBeNull();
    expect(
      screen.getByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
  });

  it("preserves the lean outer card shell while keeping the current activity region semantics", () => {
    const legendMessages = getDashboardFlowAxisLegendMessages("en");

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const card = screen.getByLabelText("Current activity");
    const legendToggle = screen.getByRole("button", {
      name: legendMessages.expandToggleLabel("graph legend"),
    });
    const legend = legendToggle.closest(
      "[data-dashboard-flow-axis-legend]",
    ) as HTMLElement | null;
    const viewport = screen.getByRole("region", {
      name: "Work graph viewport",
    });

    expect(card?.className).toContain("relative");
    expect(card?.className).toContain("flex");
    expect(card?.className).toContain("h-full");
    expect(card?.className).not.toMatch(PADDING_CLASS_PATTERN);
    expect(
      screen.getByRole("heading", { name: "Current activity" }),
    ).toBeTruthy();
    expect(screen.getByText("Observe mode")).toBeTruthy();
    expect(legend?.className).toContain("absolute");
    expect(legend?.className).toContain("left-4");
    expect(legend?.className).toContain("right-4");
    expect(legend?.className).toContain("top-4");
    expect(legend?.className).not.toMatch(PADDING_CLASS_PATTERN);
    expect(viewport.className).not.toMatch(PADDING_CLASS_PATTERN);
    expect(viewport.getAttribute("aria-describedby")).toMatch(
      /^workflow-graph-heading-/,
    );
  });

  it("renders a clear local alert when dropped PNG validation fails", async () => {
    const file = new File(["png"], "invalid-factory.png", {
      type: "image/png",
    });
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        error: {
          code: "PNG_METADATA_MISSING",
          message:
            "The selected PNG does not contain you-agent-factory factory metadata.",
        },
        ok: false,
      });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const alert = await screen.findByRole("alert");

    expect(alert.textContent).toContain("Factory import failed");
    expect(alert.textContent).toContain("invalid-factory.png");
    expect(alert.textContent).toContain(
      "This PNG does not include the you-agent-factory factory metadata needed for import.",
    );
    expect(onFactoryImportReady).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("dialog", { name: "Review factory import" }),
    ).toBeNull();
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "error",
    );
    expect(
      screen.getByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );
  });

  it("activates the dropped factory, closes the preview, and requests an active-view refresh", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    let resolveActivation: ((value: FactoryValue) => void) | null = null;
    const activateFactory = vi
      .fn<(value: FactoryValue) => Promise<FactoryValue>>()
      .mockImplementation(
        () =>
          new Promise<FactoryValue>((resolve) => {
            resolveActivation = resolve;
          }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Activate factory" }),
    );

    await waitFor(() => {
      expect(activateFactory).toHaveBeenCalledWith(importValue.factory);
    });
    const activateButton = within(previewDialog).getByRole<HTMLButtonElement>(
      "button",
      {
        name: "Activating factory...",
      },
    );
    const cancelButton = within(previewDialog).getByRole<HTMLButtonElement>(
      "button",
      {
        name: "Cancel import",
      },
    );
    const closeButton = within(previewDialog).getByRole<HTMLButtonElement>(
      "button",
      {
        name: "Close import preview",
      },
    );

    expect(activateButton.getAttribute("aria-busy")).toBe("true");
    expect(activateButton.disabled).toBe(true);
    expect(cancelButton.disabled).toBe(true);
    expect(closeButton.disabled).toBe(true);

    resolveActivation?.(importValue.factory);

    await waitFor(() => {
      expect(onFactoryActivated).toHaveBeenCalledTimes(1);
    });
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Review factory import" }),
      ).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
  });

  it("shows a distinct duplicate-name activation error without changing the current view", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateFactory = vi
      .fn<(value: FactoryValue) => Promise<FactoryValue>>()
      .mockRejectedValue(
        new NamedFactoryAPIError("Named factory already exists.", {
          code: "FACTORY_ALREADY_EXISTS",
          status: 409,
        }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Activate factory" }),
    );

    const alert = await within(previewDialog).findByRole("alert");

    expect(activateFactory).toHaveBeenCalledWith(importValue.factory);
    expect(alert.textContent).toContain("Activation failed");
    expect(alert.textContent).toContain(
      "A factory with this name already exists.",
    );
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: "Review factory import" }),
    ).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });

  it("shows a distinct non-idle activation error without changing the current view", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateFactory = vi
      .fn<(value: FactoryValue) => Promise<FactoryValue>>()
      .mockRejectedValue(
        new NamedFactoryAPIError(
          "Current factory runtime must be idle before activation.",
          {
            code: "FACTORY_NOT_IDLE",
            status: 409,
          },
        ),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Activate factory" }),
    );

    const alert = await within(previewDialog).findByRole("alert");

    expect(activateFactory).toHaveBeenCalledWith(importValue.factory);
    expect(alert.textContent).toContain("Activation failed");
    expect(alert.textContent).toContain(
      "The current factory runtime is still active.",
    );
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: "Review factory import" }),
    ).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });
});

describe("ReactFlowCurrentActivityCard graph semantics", () => {
  registerCurrentActivityCardTestLifecycle();

  it("renders active observer graph state from the canonical snapshot factory without topology fallback", async () => {
    renderCurrentActivity({ snapshot: canonicalObserverSnapshot() });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /Select .* workstation/ }),
      ).toHaveLength(5);
    });

    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.getByText("worker:agent")).toBeTruthy();
    expect(screen.getByText("worker:reviewer")).toBeTruthy();
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    expect(screen.getByText("Active Story")).toBeTruthy();
    expect(
      within(screen.getByRole("button", { name: "Select Review workstation" }))
        .getByRole("img", { name: "Repeater workstation" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("repeater");
    expect(
      within(screen.getByRole("button", { name: "Select Review workstation" }))
        .getByRole("img", { name: "Active" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("active-work");
    expect(await getStateNodeArticle("story:implemented")).toBeTruthy();
    expect(
      (await getStateNodeArticle("story:documented"))
        .querySelector("article")
        ?.className.includes("opacity-[0.45]"),
    ).toBe(true);
  });

  it("selects worker nodes from the live activity graph", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: workerDenseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: workerDenseFactoryDefinitionDocument,
      pendingFactoryDefinition: workerDenseFactoryDefinitionDocument,
    } as never);
    const snapshot = workerDenseSnapshot();
    snapshot.factory = workerDenseFactoryDefinitionDocument;

    const { onSelectWorker } = renderCurrentActivity({
      snapshot,
    });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Select writer worker" }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Select writer worker" }),
    );

    expect(onSelectWorker).toHaveBeenCalledWith("writer");
  });

  it("shows selected styling for the active worker selection", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: workerDenseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: workerDenseFactoryDefinitionDocument,
      pendingFactoryDefinition: workerDenseFactoryDefinitionDocument,
    } as never);
    const snapshot = workerDenseSnapshot();
    snapshot.factory = workerDenseFactoryDefinitionDocument;

    renderCurrentActivity({
      selection: { kind: "worker", workerName: "writer" },
      snapshot,
    });

    const workerButton = await screen.findByRole("button", {
      name: "Select writer worker",
    });
    expect(workerButton.getAttribute("aria-pressed")).toBe("true");
    expect(workerButton.getAttribute("data-selected-worker")).toBe("true");
  });

  it("keeps observer selection callbacks stable for canonical factory-backed graph nodes", async () => {
    const { onSelectStateNode, onSelectWorkID, onSelectWorkstation } =
      renderCurrentActivity({
        snapshot: canonicalObserverSnapshot(),
        selection: { kind: "state-node", placeId: "story:implemented" },
      });

    const implementedState = await screen.findByRole("button", {
      name: "Select story:implemented state",
    });
    expect(implementedState.getAttribute("data-selected-state")).toBe("true");

    fireEvent.click(
      screen.getByRole("button", { name: "Select Review workstation" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Select story:ready state" }),
    );
    fireEvent.click(screen.getByRole("button", { name: /Active Story/ }));

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");
    expect(onSelectStateNode).toHaveBeenCalledWith("story:ready");
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story", {
      dispatchID: "dispatch-review-active",
      nodeID: "review",
    });
  });

  it("renders semantic workflow activity with active, terminal, and failed graph states", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /Select .* workstation/ }),
      ).toHaveLength(5);
    });
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select story:ready state" }),
    ).toBeTruthy();
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.getByLabelText("worker:agent")).toBeTruthy();
    expect(screen.getByLabelText("work-type:story")).toBeTruthy();
    expect(
      screen
        .getAllByRole("img", { name: "Queue" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    expect(
      screen
        .getAllByRole("img", { name: "Resource" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(
      screen
        .getAllByRole("img", { name: "Worker" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("worker");
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    const reviewButton = screen.getByRole("button", {
      name: "Select Review workstation",
    });
    expect(reviewButton).toBeTruthy();
    expect(
      within(reviewButton)
        .getByRole("img", { name: "Repeater workstation" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("repeater");
    expect(
      within(reviewButton)
        .getByRole("img", { name: "Active" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("active-work");
    expect(
      (await getStateNodeArticle("story:documented"))
        .querySelector("article")
        ?.className.includes("border-af-border-strong"),
    ).toBe(true);
    expect(screen.getByText("Active Story")).toBeTruthy();
    expect(screen.queryByText("dispatch-review-active")).toBeNull();
    expect(screen.queryByText("Active Work")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select story:blocked state" }),
    ).toBeTruthy();
  });

  it("renders React Flow edges for visible graph connections", async () => {
    const reactFlowErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    try {
      renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

      expect(
        await screen.findByRole("region", { name: "Work graph viewport" }),
      ).toBeTruthy();
      await waitFor(() => {
        expect(
          document.querySelectorAll(".react-flow__edge").length,
        ).toBeGreaterThan(0);
      });
      expect(document.querySelectorAll(".react-flow__edge-path")).toHaveLength(
        document.querySelectorAll(".react-flow__edge").length,
      );
      expect(
        reactFlowErrorSpy.mock.calls.some(([firstArg]) =>
          String(firstArg).includes(
            "Couldn't create edge for source handle id",
          ),
        ),
      ).toBe(false);
    } finally {
      reactFlowErrorSpy.mockRestore();
    }
  });

  it("does not trigger React Flow missing-handle errors after entering embedded editor mode", async () => {
    const reactFlowErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);

    try {
      renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

      fireEvent.click(
        screen.getByRole("button", { name: "Enter factory graph editor" }),
      );

      await screen.findByRole("button", { name: "Connect" });
      await waitFor(() => {
        expect(document.querySelectorAll(".react-flow__edge")).not.toHaveLength(
          0,
        );
      });

      expect(
        reactFlowErrorSpy.mock.calls.some(([firstArg]) =>
          String(firstArg).includes(
            "Couldn't create edge for source handle id",
          ),
        ),
      ).toBe(false);
    } finally {
      reactFlowErrorSpy.mockRestore();
    }
  });

  it("keeps pending shared-surface editor route drafts free of handle-attachment errors", async () => {
    const reactFlowErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    const idleSnapshot = dashboardSnapshotWithActiveWorkItemCount(0);

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        edgeChanges: {
          additions: [
            {
              kind: "workstation-output",
              source: { kind: "workstation", name: "review" },
              target: {
                kind: "work-state",
                stateName: "blocked",
                workTypeName: "story",
              },
            },
          ],
          removals: [],
        },
      },
      hasChanges: true,
    } as never);

    try {
      renderCurrentActivity({ snapshot: idleSnapshot });

      fireEvent.click(
        screen.getByRole("button", { name: "Enter factory graph editor" }),
      );

      await screen.findByRole("button", { name: "Save changes" });
      expect(await screen.findByText("Unsaved graph changes")).toBeTruthy();
      expect(screen.queryByText("Topology edits are blocked")).toBeNull();
      await waitFor(() => {
        expect(document.querySelectorAll(".react-flow__edge")).not.toHaveLength(
          0,
        );
      });

      expect(
        reactFlowErrorSpy.mock.calls.some(([firstArg]) =>
          String(firstArg).includes(
            "Couldn't create edge for source handle id",
          ),
        ),
      ).toBe(false);
    } finally {
      reactFlowErrorSpy.mockRestore();
    }
  });

  it("renders every graph place family through custom React Flow node types", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        document.querySelector(
          "[data-current-activity-node-type='workstation']",
        ),
      ).toBeTruthy();
    });

    expect(
      document.querySelector(
        "[data-current-activity-node-type='statePosition']",
      ),
    ).toBeTruthy();
    expect(
      document.querySelector("[data-current-activity-node-type='resource']"),
    ).toBeTruthy();
    expect(
      document.querySelector("[data-current-activity-node-type='worker']"),
    ).toBeTruthy();
    expect(
      document.querySelector("[data-current-activity-node-type='workType']"),
    ).toBeTruthy();
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
  });

  it("keeps zero-count resources visible and readable", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({
      "agent-slot:available": 0,
    });
    renderCurrentActivity({ snapshot });

    const resourceCount = await screen.findByLabelText("0 resource tokens");
    const resourceNode = resourceCount.closest(".react-flow__node");
    const resourceArticle = resourceCount.closest("article");

    expect(resourceCount.textContent?.trim()).toBe("0");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(
      resourceArticle?.querySelector("[data-resource-name]")?.textContent,
    ).toBe("agent-slot");
    expect(
      within(resourceArticle as HTMLElement)
        .getByRole("img", { name: "Resource" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(resourceArticle?.textContent).not.toContain("Resource");
    expect(resourceNode?.getAttribute("style")).toContain("width: 168px");
    expect(resourceNode?.getAttribute("style")).toContain("height: 86px");
    expect(resourceArticle?.className).not.toContain("opacity-[0.45]");
  });

  it("renders resource, worker, and work-type role icons while preserving identifiers", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const resourceLabelContainer = await screen.findByLabelText("agent-slot");
    const resourceArticle = resourceLabelContainer.closest("article");
    const workerLabelContainer = screen.getByLabelText("worker:agent");
    const workerArticle = workerLabelContainer.closest("article");
    const workTypeLabelContainer = screen.getByLabelText("work-type:story");
    const workTypeArticle = workTypeLabelContainer.closest("article");

    expect(
      within(resourceArticle as HTMLElement)
        .getByRole("img", { name: "Resource" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(
      within(workerArticle as HTMLElement)
        .getByRole("img", { name: "Worker" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("worker");
    expect(
      within(workTypeArticle as HTMLElement)
        .getByRole("img", { name: "Work type" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("work-type");
    expect(resourceLabelContainer.getAttribute("aria-label")).toBe(
      "agent-slot",
    );
    expect(workerLabelContainer.getAttribute("aria-label")).toBe(
      "worker:agent",
    );
    expect(workTypeLabelContainer.getAttribute("aria-label")).toBe(
      "work-type:story",
    );
    expect(
      resourceArticle?.querySelector("[data-resource-name]")?.textContent,
    ).toBe("agent-slot");
    expect(resourceArticle?.textContent).not.toContain("Resource");
    expect(workerArticle?.textContent).toContain("agent");
    expect(workTypeArticle?.textContent).toContain("story");
  });

  it("renders selected-tick resource counts while active dispatches occupy and return slots", async () => {
    const idleSnapshot = resourceOccupancySnapshotForTick(1);

    expect(idleSnapshot.runtime.in_flight_dispatch_count).toBe(0);
    expect(
      idleSnapshot.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(2);

    renderCurrentActivity({ snapshot: idleSnapshot });

    const idleResourceCount = await screen.findByLabelText("2 resource tokens");

    expect(idleResourceCount.textContent?.trim()).toBe("2");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();

    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = installDashboardBrowserTestShims();

    const activeSnapshot = resourceOccupancySnapshotForTick(3);

    expect(activeSnapshot.runtime.in_flight_dispatch_count).toBe(1);
    expect(
      activeSnapshot.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(1);

    renderCurrentActivity({ snapshot: activeSnapshot });

    const activeResourceCount =
      await screen.findByLabelText("1 resource tokens");

    expect(activeResourceCount.textContent?.trim()).toBe("1");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.queryByLabelText("2 resource tokens")).toBeNull();

    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = installDashboardBrowserTestShims();

    const returnedSnapshot = resourceOccupancySnapshotForTick(4);

    expect(returnedSnapshot.runtime.in_flight_dispatch_count).toBe(0);
    expect(
      returnedSnapshot.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(2);

    renderCurrentActivity({ snapshot: returnedSnapshot });

    const returnedResourceCount =
      await screen.findByLabelText("2 resource tokens");

    expect(returnedResourceCount.textContent?.trim()).toBe("2");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.queryByLabelText("1 resource tokens")).toBeNull();
  });

  it("animates active graph flow while muting unrelated graph chrome", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("button", { name: /Active Story/ }),
    ).toBeTruthy();
    const idleStateArticle = await getStateNodeArticle("story:documented");
    const idleResourceArticle = screen
      .getByLabelText("agent-slot")
      .closest("article");
    expect(idleStateArticle.querySelector("article")?.className).toContain(
      "opacity-[0.45]",
    );
    expect(idleResourceArticle?.className).toContain("border-af-border-strong");
    expect(idleResourceArticle?.className).not.toContain("opacity-[0.45]");
  });

  it("keeps inactive and failed output paths unlabeled and out of active green flow", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveImplementWorkstation(),
    });

    expect(
      await screen.findByRole("button", {
        name: "Select Implement workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select story:blocked state" }),
    ).toBeTruthy();
    expect(screen.queryByText(/Flowing/)).toBeNull();
    expect(screen.queryByText(/Failure Path/)).toBeNull();
  });

  it("hides workstation return edges to resource nodes while keeping resource inputs visible", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithResourceReturnEdge(),
    });

    expect(await screen.findByLabelText("agent-slot")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select Implement workstation" }),
    ).toBeTruthy();
    expect(screen.getAllByLabelText("agent-slot")).toHaveLength(1);
  });

  it("uses selected accent styling over active flow styling", async () => {
    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      selection: { kind: "state-node", placeId: "story:complete" },
    });

    const activeSelectedState = await getStateNodeArticle("story:complete");
    const activeSelectedArticle = activeSelectedState.querySelector("article");

    expect(activeSelectedArticle?.className).toContain(
      "border-af-accent-border",
    );
    expect(activeSelectedArticle?.className).not.toContain(
      "border-af-success-border",
    );

    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = installDashboardBrowserTestShims();

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      selection: { kind: "node", nodeId: "review" },
    });

    const reviewButton = await screen.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewArticle = reviewButton.closest("article");

    expect(reviewArticle?.className).toContain("border-af-accent-border");
    expect(reviewArticle?.className).not.toContain("agent-flow-node--active");
  });

  it("renders the legend minimized by default and expands it for graph node and edge semantics", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const expandButton = await screen.findByRole("button", {
      name: "Expand graph legend",
    });

    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText("Graph legend")).toBeNull();

    const legend = await expandGraphLegend();
    const legendScope = within(legend);
    const collapseButton = screen.getByRole("button", {
      name: "Collapse graph legend",
    });

    expect(legendScope.getByText("Active flow")).toBeTruthy();
    expect(legendScope.getByText("Failure path")).toBeTruthy();
    expect(legend.querySelector("[data-legend-flow]")).toBeTruthy();
    expect(collapseButton.getAttribute("aria-expanded")).toBe("true");
    for (const [label, kind] of LEGEND_ICON_EXPECTATIONS) {
      const icon = legendScope.getByRole("img", {
        name: `${label} legend icon`,
      });

      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(kind);
      expect(legendScope.getByText(label)).toBeTruthy();
      expect(legend.querySelector(`[data-legend-icon='${kind}']`)).toBeTruthy();
    }
    expect(
      legend.querySelector("[data-legend-icon='queue'] span.h-3"),
    ).toBeNull();
    expect(
      legend.querySelector("[data-legend-icon='workstation'] span.border-2"),
    ).toBeNull();
    expect(
      legend.querySelector(
        "[data-legend-icon='exhaustion'] span.border-dashed",
      ),
    ).toBeNull();

    fireEvent.click(collapseButton);

    await waitFor(() => {
      expect(screen.queryByLabelText("Graph legend")).toBeNull();
    });
    expect(
      screen.getByRole("button", { name: "Expand graph legend" }),
    ).toBeTruthy();
  });

  it("does not render runtime-only exhaustion-rule topology nodes", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithExhaustionRuleNode(),
    });

    expect(
      await screen.findByRole("button", {
        name: "Select Review workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Select executor-loop-breaker exhaustion rule",
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: /Should Not Render/ }),
    ).toBeNull();
    expect(screen.queryByText("Should Not Render")).toBeNull();
  });
});

describe("ReactFlowCurrentActivityCard node layout behavior", () => {
  registerCurrentActivityCardTestLifecycle();

  it("renders the shared workstation-kind parity fixture with distinct supported icons", async () => {
    const { onSelectWorkstation } = renderCurrentActivity({
      snapshot: workstationKindParityDashboardSnapshot,
    });

    const legend = await expandGraphLegend();
    const legendScope = within(legend);

    for (const expectation of workstationKindParityExpectations) {
      const button = await screen.findByRole("button", {
        name: expectation.buttonName,
      });
      const icon = within(button).getByRole("img", {
        name: expectation.metadata.label,
      });
      const legendIcon = legendScope.getByRole("img", {
        name: `${expectation.metadata.label} legend icon`,
      });

      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(
        expectation.metadata.iconKind,
      );
      expect(button.textContent).toContain(expectation.workstationName);
      expect(button.textContent).not.toContain(expectation.metadata.label);
      expect(legendIcon.getAttribute("data-graph-semantic-icon")).toBe(
        expectation.metadata.iconKind,
      );
      expect(legendScope.getByText(expectation.metadata.label)).toBeTruthy();
    }

    const cronExpectation = workstationKindParityExpectations.find(
      (expectation) => expectation.nodeID === "nightly-cron",
    );
    const cronButton = await screen.findByRole("button", {
      name: cronExpectation?.buttonName ?? "Select Nightly Cron workstation",
    });

    expect(cronButton.getAttribute("title")).toBe("Nightly Cron");

    fireEvent.click(cronButton);

    expect(onSelectWorkstation).toHaveBeenCalledWith("nightly-cron");
  });

  it("hides system-time graph nodes from cron factories while keeping customer cron workstations selectable", async () => {
    const factory = cronSystemTimeFactoryDefinition();
    const layout = await buildCurrentActivityGraphLayoutFromFactory(factory);
    const renderedNodeIds = layout.nodes.map((node) => node.nodeId);

    expect(renderedNodeIds).not.toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
      ]),
    );
    expect(renderedNodeIds).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("workstation", "Nightly Cron"),
        systemTimeGraphNodeId("work-type", "story"),
      ]),
    );

    const { onSelectWorkstation } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithCronSystemTimeFactory(),
    });

    const cronButton = await screen.findByRole("button", {
      name: "Select Nightly Cron workstation",
    });

    expect(cronButton.getAttribute("title")).toBe("Nightly Cron");
    expect(
      screen.queryByLabelText(`work-type:${SYSTEM_TIME_WORK_TYPE_ID}`),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: `Select ${SYSTEM_TIME_EXPIRY_TRANSITION_ID} workstation`,
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: `Select ${SYSTEM_TIME_WORK_TYPE_ID}:pending state`,
      }),
    ).toBeNull();
    expect(screen.queryByText(SYSTEM_TIME_EXPIRY_TRANSITION_ID)).toBeNull();

    fireEvent.click(cronButton);

    expect(onSelectWorkstation).toHaveBeenCalledWith("nightly-cron");
  });

  it("renders state category icons without replacing state labels", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const initialStateArticle = await getStateNodeArticle("story:init");
    const processingStateArticle = await getStateNodeArticle("story:ready");
    const terminalStateArticle = await getStateNodeArticle("story:complete");
    const failedStateArticle = await getStateNodeArticle("story:blocked");

    expect(
      within(initialStateArticle)
        .getByRole("img", { name: "Queue" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    expect(
      within(processingStateArticle)
        .getByRole("img", { name: "Queue" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    expect(
      within(terminalStateArticle)
        .getByRole("img", { name: "Queue" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    expect(
      within(failedStateArticle)
        .getByRole("img", { name: "Queue" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    expect(
      initialStateArticle.querySelector("[data-state-work-type]")?.textContent,
    ).toBe("story");
    expect(
      initialStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("init");
    expect(
      processingStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("ready");
    expect(
      terminalStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("complete");
    expect(
      failedStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("blocked");
    expect(
      initialStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Queue");
    expect(
      terminalStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Terminal");
    expect(
      failedStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Failed");
    expect(
      failedStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Queue");
  });

  it("themes the React Flow controls with dashboard colors", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    const controls = document.querySelector<HTMLElement>(
      ".react-flow__controls",
    );
    const zoomIn = controls?.querySelector<HTMLButtonElement>(
      ".react-flow__controls-zoomin",
    );

    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-button-background-color-props: var(--color-af-graph-controls-button-surface)",
    );
    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-button-color-props: var(--color-af-graph-controls-text)",
    );
    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-box-shadow: none",
    );
    expect(controls?.getAttribute("style")).not.toContain("#fefefe");
    expect(zoomIn).toBeTruthy();
  });

  it("renders state-position markers as green dots for low-count active states", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 3 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const dotContainer = readyStateArticle.querySelector(
      "[data-state-work-progress='dots']",
    );
    const dots = readyStateArticle.querySelectorAll(
      "[data-state-work-progress-dot]",
    );
    const dotIndices = Array.from(dots).map((dot) =>
      dot.getAttribute("data-state-work-progress-dot"),
    );

    expect(dotContainer).toBeTruthy();
    expect(dotIndices).toEqual(["0", "1", "2"]);
    expect(dotContainer?.getAttribute("aria-label")).toBe("3 active items");
    expect(
      within(readyStateArticle).getByLabelText("3 active items"),
    ).toBeTruthy();
  });

  it("renders work-state labels and markers in separated stable zones", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 3 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const labelZone = readyStateArticle.querySelector(
      "[data-state-label-zone]",
    );
    const markerZone = readyStateArticle.querySelector(
      "[data-state-marker-zone]",
    );
    const workType = readyStateArticle.querySelector("[data-state-work-type]");
    const stateValue = readyStateArticle.querySelector("[data-state-value]");

    expect(labelZone).toBeTruthy();
    expect(markerZone).toBeTruthy();
    expect(workType?.textContent).toBe("story");
    expect(stateValue?.textContent).toBe("ready");
    expect(labelZone?.textContent).not.toContain(":");
    expect(labelZone?.textContent).not.toContain("3 active items");
    expect(within(readyStateArticle).queryByText("story:ready")).toBeNull();
    expect(markerZone?.textContent).not.toContain("story");
    expect(
      markerZone?.querySelectorAll("[data-state-work-progress-dot]"),
    ).toHaveLength(3);
  });

  it("renders exactly 10 state-position markers in a compact ordered grid", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 10 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const dotContainer = readyStateArticle.querySelector(
      "[data-state-work-progress='dots']",
    );
    const dotIndices = Array.from(
      readyStateArticle.querySelectorAll("[data-state-work-progress-dot]"),
    ).map((dot) => dot.getAttribute("data-state-work-progress-dot"));

    expect(dotContainer?.className).toContain("grid-cols-[repeat(5,0.5rem)]");
    expect(dotIndices).toEqual([
      "0",
      "1",
      "2",
      "3",
      "4",
      "5",
      "6",
      "7",
      "8",
      "9",
    ]);
  });

  it("uses numeric fallback for state-position active counts above 10", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 11 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const numeric = readyStateArticle.querySelector(
      "[data-state-work-progress='numeric']",
    );

    expect(numeric).toBeTruthy();
    expect(numeric?.textContent?.trim()).toBe("11");
    expect(numeric?.getAttribute("aria-label")).toBe("11 active items");
    expect(
      readyStateArticle.querySelector("[data-state-work-progress='dots']"),
    ).toBeNull();
  });

  it("keeps long work-state labels bounded inside the state label zone", async () => {
    renderCurrentActivity({ snapshot: dashboardSnapshotWithLongStateLabels() });
    const longStateButton = await screen.findByRole("button", {
      name: "Select customer-escalation-story-with-a-deliberately-long-type:ready-for-review-after-multiple-dependent-checks-complete state",
    });
    const longStateNode = longStateButton.closest(".react-flow__node");
    const labelZone = longStateNode?.querySelector("[data-state-label-zone]");
    const workType = longStateNode?.querySelector("[data-state-work-type]");
    const stateValue = longStateNode?.querySelector("[data-state-value]");
    const markerZone = longStateNode?.querySelector("[data-state-marker-zone]");

    expect(longStateNode?.getAttribute("style")).toContain("width: 164px");
    expect(longStateNode?.getAttribute("style")).toContain("height: 86px");
    expect(longStateButton.className).toContain("flex-col");
    expect(longStateButton.className).toContain("overflow-hidden");
    expect(labelZone?.className).toContain("h-6");
    expect(labelZone?.className).toContain("max-h-6");
    expect(labelZone?.className).toContain("overflow-hidden");
    expect(workType?.className).toContain("text-ellipsis");
    expect(workType?.getAttribute("title")).toBe(
      "customer-escalation-story-with-a-deliberately-long-type",
    );
    expect(stateValue?.className).toContain("overflow-hidden");
    expect(stateValue?.className).toContain("truncate");
    expect(stateValue?.className).toContain("whitespace-nowrap");
    expect(stateValue?.getAttribute("title")).toBe(
      "ready-for-review-after-multiple-dependent-checks-complete",
    );
    expect(markerZone).toBeTruthy();
    expect(markerZone?.className).toContain("shrink-0");
    expect(markerZone?.getAttribute("title")).toBe(
      "customer-escalation-story-with-a-deliberately-long-type:ready-for-review-after-multiple-dependent-checks-complete",
    );
  });

  it("applies green in-progress state styling only when active items are > 0", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({
      "story:ready": 4,
      "story:documented": 0,
    });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const documentedStateArticle =
      await getStateNodeArticle("story:documented");

    expect(readyStateArticle.querySelector("article")?.className).toContain(
      "border-af-border-strong",
    );
    expect(
      documentedStateArticle.querySelector("article")?.className,
    ).toContain("border-af-border-strong");
  });

  it("selects workstation and work item context through the dashboard callbacks", async () => {
    const { onSelectWorkID, onSelectWorkstation } = renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    );

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    await waitFor(() => {
      expect(onSelectWorkID).toHaveBeenCalled();
    });
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story", {
      dispatchID: "dispatch-review-active",
      nodeID: "review",
    });
  });

  it("caps workstation work item names at three and summarizes the rest", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(5),
    });

    expect(
      await screen.findByRole("button", { name: /Active Story 1/ }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Active Story 3/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Active Story 4/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Active Story 5/ })).toBeNull();
    expect(screen.getByLabelText("5 active items")).toBeTruthy();
    expect(screen.getAllByText("+2")).toHaveLength(1);
  });

  it("keeps workstation height stable while summarizing more than three active items", async () => {
    const { onSelectWorkstation } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(6),
    });

    const reviewButton = await screen.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewNode = reviewButton.closest(".react-flow__node");
    const dots = reviewNode?.querySelector(
      "[data-workstation-work-progress='dots']",
    );

    expect(dots).toBeTruthy();
    expect(dots?.getAttribute("aria-label")).toBe("6 active items");
    expectFixedWorkstationNodeDimensions(reviewNode);
    expect(screen.getByRole("button", { name: /Active Story 1/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Active Story 3/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Active Story 4/ })).toBeNull();
    expect(reviewNode?.querySelector("article")?.className).toContain(
      "border-af-success-border",
    );

    fireEvent.click(reviewButton);

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");
  });

  it("keeps workstation node dimensions fixed across zero, one, five, and six active items", async () => {
    const { rerender } = renderWithQueryClient(
      <ReactFlowCurrentActivityCard
        now={Date.parse("2026-04-08T12:00:04Z")}
        onSelectWorkID={vi.fn()}
        selection={null}
        snapshot={dashboardSnapshotWithActiveWorkItemCount(0)}
        onSelectStateNode={vi.fn()}
        onSelectWorker={vi.fn()}
        onSelectWorkstation={vi.fn()}
      />,
    );

    for (const activeItemCount of [0, 1, 5, 6]) {
      rerender(
        <QueryClientProvider
          client={
            new QueryClient({
              defaultOptions: {
                mutations: { retry: false },
                queries: { gcTime: Infinity, retry: false },
              },
            })
          }
        >
          <ReactFlowCurrentActivityCard
            now={Date.parse("2026-04-08T12:00:04Z")}
            onSelectWorkID={vi.fn()}
            selection={null}
            snapshot={dashboardSnapshotWithActiveWorkItemCount(activeItemCount)}
            onSelectStateNode={vi.fn()}
            onSelectWorker={vi.fn()}
            onSelectWorkstation={vi.fn()}
          />
        </QueryClientProvider>,
      );

      const reviewNode = await getWorkstationNode();

      expectFixedWorkstationNodeDimensions(reviewNode);
    }
  });

  it("keeps workstation position keys stable when selected ticks change active work counts", async () => {
    const zeroActiveSnapshot = dashboardSnapshotWithActiveWorkItemCount(0);
    const sixActiveSnapshot = dashboardSnapshotWithActiveWorkItemCount(6);
    if (!zeroActiveSnapshot.factory || !sixActiveSnapshot.factory) {
      throw new Error("expected active-count fixtures to include factories");
    }
    const zeroActiveLayout = await buildCurrentActivityGraphLayoutFromFactory(
      zeroActiveSnapshot.factory,
    );
    const sixActiveLayout = await buildCurrentActivityGraphLayoutFromFactory(
      sixActiveSnapshot.factory,
    );
    const graphKey = currentActivityGraphKey(zeroActiveLayout);

    expect(currentActivityGraphKey(sixActiveLayout)).toBe(graphKey);

    useCurrentActivityGraphStore
      .getState()
      .setNodePosition(graphKey, "workstation:review", { x: 321, y: 654 });

    const callbacks = {
      onSelectWorkID: vi.fn(),
      onSelectStateNode: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
    };
    const { rerender } = renderWithQueryClient(
      <ReactFlowCurrentActivityCard
        now={Date.parse("2026-04-08T12:00:04Z")}
        selection={null}
        snapshot={zeroActiveSnapshot}
        {...callbacks}
      />,
    );

    let reviewNode = await getWorkstationNode();
    expectFixedWorkstationNodeDimensions(reviewNode);

    rerender(
      <QueryClientProvider
        client={
          new QueryClient({
            defaultOptions: {
              mutations: { retry: false },
              queries: { gcTime: Infinity, retry: false },
            },
          })
        }
      >
        <ReactFlowCurrentActivityCard
          now={Date.parse("2026-04-08T12:00:04Z")}
          selection={null}
          snapshot={sixActiveSnapshot}
          {...callbacks}
        />
      </QueryClientProvider>,
    );

    reviewNode = await getWorkstationNode();
    expectFixedWorkstationNodeDimensions(reviewNode);
  });
});

describe("ReactFlowCurrentActivityCard topology selection and localization", () => {
  registerCurrentActivityCardTestLifecycle();

  it("derives a stable topology cache key for equivalent cloned workflow topology", () => {
    const firstKey = currentActivityTopologyKey(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const secondKey = currentActivityTopologyKey(
      structuredClone(semanticWorkflowDashboardSnapshot).topology,
    );

    expect(secondKey).toBe(firstKey);
  });

  it("selects work-state nodes without making resource nodes selectable", async () => {
    const { onSelectStateNode } = renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      selection: { kind: "state-node", placeId: "story:implemented" },
    });

    const stateButton = await screen.findByRole("button", {
      name: "Select story:implemented state",
    });

    expect(stateButton.getAttribute("aria-pressed")).toBe("true");
    expect(stateButton.getAttribute("data-selected-state")).toBe("true");
    expect(
      screen.queryByRole("button", {
        name: "Select agent-slot state",
      }),
    ).toBeNull();

    fireEvent.click(
      await screen.findByRole("button", { name: "Select story:ready state" }),
    );

    expect(onSelectStateNode).toHaveBeenCalledWith("story:ready");
  });

  it("keeps long workstation and active work labels from hiding the duration", async () => {
    const labels = [
      "Short Active Story",
      "Active Story With A Medium Sized Label",
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    ];
    const { onSelectWorkID } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(),
    });
    const longWorkstationButton = await screen.findByRole("button", {
      name: /Select Review Requests With A Deliberately Long Workstation Title workstation/,
    });
    const longWorkstationLabel = longWorkstationButton.querySelector(
      "[data-workstation-title]",
    );
    const longWorkButton = await screen.findByRole("button", {
      name: /Active Story With A Deliberately Long Label/,
    });
    const longWorkLabel = longWorkButton.querySelector(
      "[data-active-work-label]",
    );
    const durationLabel = longWorkButton.querySelector(
      "[data-active-work-duration]",
    );
    const reviewNode = longWorkButton.closest(".react-flow__node");

    expect(reviewNode?.getAttribute("style")).toContain("width: 156px");
    expect(longWorkstationButton.getAttribute("title")).toBe(
      "Review Requests With A Deliberately Long Workstation Title",
    );
    expect(longWorkstationLabel?.className).toContain("truncate");
    expect(longWorkstationLabel?.className).toContain("whitespace-nowrap");
    expect(longWorkButton.className).toContain("min-w-0");
    expect(longWorkButton.className).toContain(
      "grid-cols-[minmax(0,1fr)_auto]",
    );
    expect(longWorkButton.className).toContain("overflow-hidden");
    expect(longWorkLabel?.className).toContain("truncate");
    expect(longWorkLabel?.className).toContain("basis-0");
    expect(durationLabel?.textContent).toBe("4s");
    expect(durationLabel?.className).toContain("whitespace-nowrap");
    expect(durationLabel?.className).toContain("text-right");
    expect(durationLabel?.className).not.toContain("overflow-hidden");
    labels.forEach((label) => {
      const activeWorkButton = screen.getByRole("button", {
        name: new RegExp(label),
      });
      const labelElement = activeWorkButton.querySelector(
        "[data-active-work-label]",
      );
      const durationElement = activeWorkButton.querySelector(
        "[data-active-work-duration]",
      );

      expect(labelElement?.textContent).toBe(label);
      expect(labelElement?.className).toContain("min-w-0");
      expect(labelElement?.className).toContain("truncate");
      expect(durationElement?.textContent).toBe("4s");
      expect(durationElement?.className).toContain("shrink-0");
      expect(durationElement?.className).toContain("whitespace-nowrap");
    });
    expect(longWorkButton.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(longWorkButton);

    await waitFor(() => {
      expect(onSelectWorkID).toHaveBeenCalled();
    });
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story-3", {
      dispatchID: "dispatch-review-active",
      nodeID: "review",
    });

    cleanup();
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(),
      selection: {
        dispatchId: "dispatch-review-active",
        kind: "work-item",
        nodeId: "review",
        workID: "work-active-story-3",
      },
    });

    expect(
      (
        await screen.findByRole("button", {
          name: /Active Story With A Deliberately Long Label/,
        })
      ).getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("renders a safe fallback label when an active work item is missing both display name and work id", async () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.[
        "dispatch-review-active"
      ];

    if (!activeExecution) {
      throw new Error(
        "expected semantic workflow fixture to include an active review execution",
      );
    }

    activeExecution.work_items = [
      {
        trace_id: "trace-malformed-active-story",
        work_type_id: "story",
      } as DashboardWorkItemRef,
    ];
    activeExecution.trace_ids = ["trace-malformed-active-story"];

    renderCurrentActivity({ snapshot });

    expect(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Unknown work/ })).toBeTruthy();
  });

  it("renders a single-node workflow without edge data", async () => {
    renderCurrentActivity({ snapshot: singleNodeDashboardSnapshot });

    expect(
      await screen.findByRole("button", { name: "Select Intake workstation" }),
    ).toBeTruthy();
    expect(screen.queryByText("Idle")).toBeNull();
  });

  it("renders a twenty-node workflow fixture for larger graphs", async () => {
    const legendMessages = getDashboardFlowAxisLegendMessages("en");

    renderCurrentActivity({ snapshot: twentyNodeDashboardSnapshot });

    expect(
      await screen.findByRole("button", {
        name: "Select Station 20 workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.getAllByRole("button", { name: /Select .* workstation/ }),
    ).toHaveLength(20);
    expect(
      screen.getAllByRole("img", { name: "Standard workstation" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen
        .getAllByRole("img", { name: "Queue" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    const _legend = await expandGraphLegend();
    expect(
      within(screen.getByLabelText(legendMessages.title))
        .getByRole("img", {
          name: legendMessages.iconLabel(legendMessages.iconLabels.workstation),
        })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("workstation");
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select story:step-6 state" }),
    ).toBeTruthy();
  });

  it("renders localized legend copy for the workflow graph without changing the graph interactions", async () => {
    const messages = getDashboardFlowAxisLegendMessages("ja");

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      locale: "ja",
    });

    const expandButton = await screen.findByRole("button", {
      name: messages.expandToggleLabel(messages.title),
    });

    fireEvent.click(expandButton);

    const legend = await screen.findByLabelText(messages.title);

    expect(
      within(legend).getByText(messages.edgeLabels.activeFlow),
    ).toBeTruthy();
    expect(
      within(legend)
        .getByRole("img", {
          name: messages.iconLabel(messages.iconLabels.workstation),
        })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("workstation");
    expect(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
  });

  it("renders localized graph-import overlay and preview dialog shell copy without changing import behavior", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const previewMessages = getImportPreviewDialogMessages("ja");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      locale: "ja",
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(screen.getByText(graphMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(graphMessages.graphDropHint)).toBeTruthy();

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: previewMessages.title,
    });

    expect(
      within(previewDialog).getByRole("button", {
        name: previewMessages.closeLabel,
      }),
    ).toBeTruthy();
  });

  it("renders localized graph-import validation errors and preserves dismiss reset behavior", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const file = new File(["png"], "invalid-factory.png", {
      type: "image/png",
    });
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        error: {
          code: "PNG_METADATA_MISSING",
          message:
            "The selected PNG does not contain you-agent-factory factory metadata.",
        },
        ok: false,
      });

    renderCurrentActivity({
      locale: "ja",
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const alert = await screen.findByRole("alert");

    expect(alert.textContent).toContain(graphMessages.graphImportErrorTitle);
    expect(alert.textContent).toContain("invalid-factory.png");
    expect(alert.textContent).toContain(
      graphMessages.importErrorMetadataMissing,
    );
    expect(onFactoryImportReady).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("dialog", { name: "Review factory import" }),
    ).toBeNull();
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "error",
    );

    fireEvent.click(
      screen.getByRole("button", { name: graphMessages.dismissAction }),
    );

    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );
  });

  it("keeps localized preview dialog shell controls available when activation fails", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const previewMessages = getImportPreviewDialogMessages("ja");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateFactory = vi
      .fn<(value: FactoryValue) => Promise<FactoryValue>>()
      .mockRejectedValue(
        new NamedFactoryAPIError("Named factory already exists.", {
          code: "FACTORY_ALREADY_EXISTS",
          status: 409,
        }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      locale: "ja",
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: previewMessages.title,
    });

    expect(
      within(previewDialog).getByText(graphMessages.dialogFlowLabel),
    ).toBeTruthy();

    fireEvent.click(
      within(previewDialog).getByRole("button", {
        name: previewMessages.activateAction,
      }),
    );

    const alert = await within(previewDialog).findByRole("alert");

    expect(alert.textContent).toContain(previewMessages.activationErrorTitle);
    expect(alert.textContent).toContain(
      previewMessages.errorByCode.FACTORY_ALREADY_EXISTS,
    );
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: previewMessages.title }),
    ).toBeTruthy();
    expect(
      within(previewDialog).getByRole("button", {
        name: previewMessages.closeLabel,
      }),
    ).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });

  it("falls back to English graph-import copy for unsupported locales", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("en");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });

    renderCurrentActivity({
      locale: "fr-CA",
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(screen.getByText(graphMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(graphMessages.graphDropHint)).toBeTruthy();
  });

  it(
    "falls back to English legend copy for unsupported workflow graph locales",
    async () => {
      const messages = getDashboardFlowAxisLegendMessages("en");

      renderCurrentActivity({
        snapshot: semanticWorkflowDashboardSnapshot,
        locale: "fr-CA",
      });

      const legend = await expandGraphLegend("fr-CA");

      expect(
        within(legend).getByText(messages.edgeLabels.activeFlow),
      ).toBeTruthy();
      expect(
        within(legend)
          .getByRole("img", {
            name: messages.iconLabel(messages.iconLabels.queue),
          })
          .getAttribute("data-graph-semantic-icon"),
      ).toBe("queue");
      expect(
        await screen.findByRole("button", {
          name: "Select Review workstation",
        }),
      ).toBeTruthy();
    },
    workflowGraphLocaleFallbackTimeoutMs,
  );

  it("derives persisted graph node keys from the canonical factory graph", async () => {
    if (!semanticWorkflowDashboardSnapshot.factory) {
      throw new Error(
        "expected semantic workflow fixture to include a factory",
      );
    }
    const layout = await buildCurrentActivityGraphLayoutFromFactory(
      semanticWorkflowDashboardSnapshot.factory,
    );
    const graphKey = currentActivityGraphKey(layout);

    useCurrentActivityGraphStore
      .getState()
      .setNodePosition(graphKey, "workstation:review", { x: 777, y: 333 });
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const reviewButton = await screen.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewNode = reviewButton.closest(".react-flow__node");

    expect(reviewNode?.getAttribute("style")).toContain("width: 156px");
    expect(
      useCurrentActivityGraphStore.getState().positionsByGraphKey[graphKey],
    ).toEqual(
      expect.objectContaining({
        "workstation:review": { x: 777, y: 333 },
      }),
    );
  });
});
