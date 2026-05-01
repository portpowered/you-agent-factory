import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactElement } from "react";
import { buildGraphLayout } from "../flowchart/layout";
import {
  EXHAUSTION_WORKSTATION_ICON_METADATA,
  SUPPORTED_WORKSTATION_ICON_METADATA,
} from "../flowchart";
import {
  ReactFlowCurrentActivityCard,
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card";
import {
  NamedFactoryAPIError,
  type NamedFactoryValue,
} from "../../api/named-factory";
import { installDashboardBrowserTestShims } from "../../components/dashboard/test-browser-shims";
import type { FactoryPngImportValue, ReadFactoryImportFile } from "../import";
import {
  resourceOccupancySnapshotForTick,
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
  workstationKindParityExpectations,
  workstationKindParityDashboardSnapshot,
} from "../../components/dashboard/test-fixtures";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import type {
  DashboardActiveExecution,
  DashboardPlaceRef,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import { useCurrentActivityGraphStore } from "../../state/currentActivityGraphStore";

interface RenderCurrentActivityOptions {
  activateNamedFactory?: (value: NamedFactoryValue) => Promise<NamedFactoryValue>;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  snapshot: DashboardSnapshot;
  selection?: CurrentActivitySelection | null;
}

const LEGEND_ICON_EXPECTATIONS = [
  ["Queue", "queue"],
  ["Processing", "processing"],
  ["Terminal", "terminal"],
  ["Failed state", "failed"],
  ["Resource", "resource"],
  ["Constraint", "constraint"],
  ["Limit", "limit"],
  ...SUPPORTED_WORKSTATION_ICON_METADATA.map((metadata) => [metadata.label, metadata.iconKind]),
  ["Active work", "active-work"],
  [
    EXHAUSTION_WORKSTATION_ICON_METADATA.label,
    EXHAUSTION_WORKSTATION_ICON_METADATA.iconKind,
  ],
] as const;

function dashboardSnapshotWithStateCounts(overrides: Record<string, number>): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.runtime.place_token_counts = {
    ...snapshot.runtime.place_token_counts,
    ...overrides,
  };

  return snapshot;
}

async function getStateNodeArticle(
  label: string,
): Promise<HTMLElement> {
  const button = await screen.findByRole("button", {
    name: `Select ${label} state`,
  });
  return button.closest(".react-flow__node") as HTMLElement;
}

async function getWorkstationNode(
  label = "Review",
): Promise<HTMLElement> {
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
  activateNamedFactory,
  onFactoryActivated,
  onFactoryImportReady,
  readFactoryImportFile,
  snapshot,
  selection = null,
}: RenderCurrentActivityOptions) {
  const onSelectWorkItem = vi.fn<
    (
      dispatchId: string,
      nodeId: string,
      execution: DashboardActiveExecution,
      workItem: DashboardWorkItemRef,
    ) => void
  >();
  const onSelectStateNode = vi.fn<(placeId: string) => void>();
  const onSelectWorkstation = vi.fn<(nodeId: string) => void>();

  renderWithQueryClient(
    <ReactFlowCurrentActivityCard
      activateNamedFactory={activateNamedFactory}
      now={Date.parse("2026-04-08T12:00:04Z")}
      onFactoryActivated={onFactoryActivated}
      onFactoryImportReady={onFactoryImportReady}
      onSelectStateNode={onSelectStateNode}
      onSelectWorkItem={onSelectWorkItem}
      onSelectWorkstation={onSelectWorkstation}
      readFactoryImportFile={readFactoryImportFile}
      selection={selection}
      snapshot={snapshot}
    />,
  );

  return { onSelectStateNode, onSelectWorkItem, onSelectWorkstation };
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

  return render(<QueryClientProvider client={queryClient}>{view}</QueryClientProvider>);
}

function createFactoryImportValue(): FactoryPngImportValue {
  return {
    envelope: {
      factory: {
        workTypes: [],
        workers: [],
        workstations: [],
      },
      name: "Dropped Factory",
      schemaVersion: "portos.agent-factory.png.v1",
    },
    factory: {
      workTypes: [],
      workers: [],
      workstations: [],
    },
    factoryName: "Dropped Factory",
    namedFactory: {
      factory: {
        workTypes: [],
        workers: [],
        workstations: [],
      },
      name: "Dropped Factory",
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: vi.fn(),
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

async function expandGraphLegend(): Promise<HTMLElement> {
  const expandButton = await screen.findByRole("button", { name: "Expand graph legend" });

  fireEvent.click(expandButton);

  return await screen.findByLabelText("Graph legend");
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
    snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
  const reviewWorkstation = snapshot.topology.workstation_nodes_by_id.review;

  if (reviewWorkstation && workstationName) {
    reviewWorkstation.workstation_name = workstationName;
  }

  if (activeExecution) {
    activeExecution.work_items = labels.map((label, index): DashboardWorkItemRef => {
      const itemNumber = index + 1;

      return {
        display_name: label,
        trace_id: `trace-active-story-${itemNumber}`,
        work_id: `work-active-story-${itemNumber}`,
        work_type_id: "story",
      };
    });
    activeExecution.trace_ids = activeExecution.work_items.map(
      (workItem) => workItem.trace_id ?? workItem.work_id,
    );
  }

  return snapshot;
}

function dashboardSnapshotWithLongStateLabels(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  for (const workstation of Object.values(snapshot.topology.workstation_nodes_by_id)) {
    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      if (place.place_id === "story:ready") {
        place.type_id = "customer-escalation-story-with-a-deliberately-long-type";
        place.state_value = "ready-for-review-after-multiple-dependent-checks-complete";
      }
    }
  }

  return snapshot;
}

function dashboardSnapshotWithActiveWorkItemCount(count: number): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const reviewActivity = snapshot.runtime.workstation_activity_by_node_id?.review;
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
  const workItems = Array.from({ length: count }, (_, index): DashboardWorkItemRef => {
    const itemNumber = index + 1;

    return {
      display_name: `Active Story ${itemNumber}`,
      trace_id: `trace-active-story-${itemNumber}`,
      work_id: `work-active-story-${itemNumber}`,
      work_type_id: "story",
    };
  });

  if (count === 0) {
    snapshot.runtime.active_dispatch_ids = (snapshot.runtime.active_dispatch_ids ?? []).filter(
      (dispatchID) => dispatchID !== "dispatch-review-active",
    );
    snapshot.runtime.active_workstation_node_ids = (
      snapshot.runtime.active_workstation_node_ids ?? []
    ).filter((nodeID) => nodeID !== "review");
    if (snapshot.runtime.active_executions_by_dispatch_id) {
      delete snapshot.runtime.active_executions_by_dispatch_id["dispatch-review-active"];
    }
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = [];
      reviewActivity.active_work_items = [];
      reviewActivity.trace_ids = [];
    }
  } else if (activeExecution) {
    activeExecution.work_items = workItems;
    activeExecution.trace_ids = workItems.map((workItem) => workItem.trace_id ?? workItem.work_id);
    snapshot.runtime.active_dispatch_ids = ["dispatch-review-active"];
    snapshot.runtime.active_workstation_node_ids = ["review"];
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = ["dispatch-review-active"];
      reviewActivity.active_work_items = workItems;
      reviewActivity.trace_ids = workItems.map((workItem) => workItem.trace_id ?? workItem.work_id);
    }
  }

  snapshot.runtime.in_flight_dispatch_count = count;

  return snapshot;
}

function dashboardSnapshotWithActiveImplementWorkstation(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
  const implementWorkstation = snapshot.topology.workstation_nodes_by_id.implement;

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

  return snapshot;
}

function dashboardSnapshotWithResourceReturnEdge(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const implementWorkstation = snapshot.topology.workstation_nodes_by_id.implement;
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

  return snapshot;
}

function dashboardSnapshotWithLimitPlace(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const implementWorkstation = snapshot.topology.workstation_nodes_by_id.implement;
  const rateLimitPlace: DashboardPlaceRef = {
    kind: "limit",
    place_id: "rate-limit:available",
    state_value: "available",
    type_id: "rate-limit",
  };

  if (implementWorkstation) {
    implementWorkstation.input_places = [
      ...(implementWorkstation.input_places ?? []),
      rateLimitPlace,
    ];
    implementWorkstation.input_place_ids = [
      ...(implementWorkstation.input_place_ids ?? []),
      rateLimitPlace.place_id,
    ];
  }

  snapshot.runtime.place_token_counts = {
    ...(snapshot.runtime.place_token_counts ?? {}),
    [rateLimitPlace.place_id]: 1,
  };

  return snapshot;
}

function dashboardSnapshotWithExhaustionRuleNode(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  snapshot.topology.workstation_node_ids = [
    ...snapshot.topology.workstation_node_ids,
    "executor-loop-breaker",
  ];
  snapshot.topology.workstation_nodes_by_id["executor-loop-breaker"] = {
    input_place_ids: ["story:ready"],
    input_places: [{
      kind: "work_state",
      place_id: "story:ready",
      state_category: "PROCESSING",
      state_value: "ready",
      type_id: "story",
    }],
    node_id: "executor-loop-breaker",
    output_place_ids: ["story:blocked"],
    output_places: [{
      kind: "work_state",
      place_id: "story:blocked",
      state_category: "FAILED",
      state_value: "blocked",
      type_id: "story",
    }],
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
      work_items: [{
        display_name: "Should Not Render",
        trace_id: "trace-hidden-exhaustion",
        work_id: "work-hidden-exhaustion",
        work_type_id: "story",
      }],
    },
  };

  return snapshot;
}

let restoreBrowserTestShims: (() => void) | null = null;

describe("ReactFlowCurrentActivityCard", () => {
  beforeEach(() => {
    window.localStorage.clear();
    useCurrentActivityGraphStore.setState({ positionsByGraphKey: {} });
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });

  it("scopes file drag-over and drop handling to the graph viewport and opens a preview", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady = vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockResolvedValue({
      ok: true,
      value: importValue,
    });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", { name: "Work graph viewport" });
    const legendToggle = screen.getByRole("button", { name: "Expand graph legend" });

    fireEvent.dragOver(legendToggle, createFileDropTransfer([file]));

    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe("idle");
    expect(readFactoryImportFile).not.toHaveBeenCalled();

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe("drag-active");
    expect(screen.getByText("Import factory PNG")).toBeTruthy();
    expect(screen.getByText("Drop a Port OS factory PNG onto this graph to start import.")).toBeTruthy();

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    await waitFor(() => {
      expect(readFactoryImportFile).toHaveBeenCalledWith(file);
    });
    await waitFor(() => {
      expect(onFactoryImportReady).toHaveBeenCalledWith(importValue, file);
    });
    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");
    expect(previewDialog.textContent).toContain(
      "Review the dropped factory before activation.",
    );
    expect(
      within(previewDialog).getByRole("img", { name: "Dropped Factory preview image" })
        .getAttribute("src"),
    )
      .toBe("blob:factory-preview");
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe("idle");

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Cancel import" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Select Review workstation" })).toBeTruthy();
  });

  it("renders a clear local alert when dropped PNG validation fails", async () => {
    const file = new File(["png"], "invalid-factory.png", { type: "image/png" });
    const onFactoryImportReady = vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockResolvedValue({
      error: {
        code: "PNG_METADATA_MISSING",
        message: "The selected PNG does not contain Port OS factory metadata.",
      },
      ok: false,
    });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", { name: "Work graph viewport" });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const alert = await screen.findByRole("alert");

    expect(alert.textContent).toContain("Factory import failed");
    expect(alert.textContent).toContain("invalid-factory.png");
    expect(alert.textContent).toContain(
      "This PNG does not include the Port OS factory metadata needed for import.",
    );
    expect(onFactoryImportReady).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe("error");
    expect(screen.getByRole("button", { name: "Select Review workstation" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe("idle");
  });

  it("activates the dropped factory, closes the preview, and requests an active-view refresh", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateNamedFactory = vi.fn<(value: NamedFactoryValue) => Promise<NamedFactoryValue>>()
      .mockResolvedValue({
        factory: importValue.factory,
        name: importValue.factoryName,
      });
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockResolvedValue({
      ok: true,
      value: importValue,
    });

    renderCurrentActivity({
      activateNamedFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", { name: "Work graph viewport" });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

    await waitFor(() => {
      expect(activateNamedFactory).toHaveBeenCalledWith({
        factory: importValue.factory,
        name: "Dropped Factory",
      });
    });
    await waitFor(() => {
      expect(onFactoryActivated).toHaveBeenCalledTimes(1);
    });
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
  });

  it("shows a distinct duplicate-name activation error without changing the current view", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateNamedFactory = vi.fn<(value: NamedFactoryValue) => Promise<NamedFactoryValue>>()
      .mockRejectedValue(
        new NamedFactoryAPIError("Named factory already exists.", {
          code: "FACTORY_ALREADY_EXISTS",
          status: 409,
        }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockResolvedValue({
      ok: true,
      value: importValue,
    });

    renderCurrentActivity({
      activateNamedFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", { name: "Work graph viewport" });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

    const alert = await within(previewDialog).findByRole("alert");

    expect(alert.textContent).toContain("Activation failed");
    expect(alert.textContent).toContain("A factory with this name already exists.");
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: "Review factory import" })).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });

  it("shows a distinct non-idle activation error without changing the current view", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateNamedFactory = vi.fn<(value: NamedFactoryValue) => Promise<NamedFactoryValue>>()
      .mockRejectedValue(
        new NamedFactoryAPIError("Current factory runtime must be idle before activation.", {
          code: "FACTORY_NOT_IDLE",
          status: 409,
        }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockResolvedValue({
      ok: true,
      value: importValue,
    });

    renderCurrentActivity({
      activateNamedFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", { name: "Work graph viewport" });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

    const alert = await within(previewDialog).findByRole("alert");

    expect(alert.textContent).toContain("Activation failed");
    expect(alert.textContent).toContain("The current factory runtime is still active.");
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: "Review factory import" })).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });

  it("renders semantic workflow activity with active, terminal, and failed graph states", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(await screen.findByRole("region", { name: "Work graph viewport" })).toBeTruthy();
    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /Select .* workstation/ })).toHaveLength(5);
    });
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
    expect(screen.getByRole("button", { name: "Select story:ready state" })).toBeTruthy();
    expect(screen.getByLabelText("agent-slot:available")).toBeTruthy();
    expect(screen.getByText("quality-gate:ready")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Queue" }).getAttribute("data-graph-semantic-icon"))
      .toBe("queue");
    expect(screen.getByRole("img", { name: "Resource" }).getAttribute("data-graph-semantic-icon"))
      .toBe("resource");
    expect(screen.getByRole("img", { name: "Constraint" }).getAttribute("data-graph-semantic-icon"))
      .toBe("constraint");
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    expect(screen.getByLabelText("1 constraint token")).toBeTruthy();
    const reviewButton = screen.getByRole("button", { name: "Select Review workstation" });
    expect(reviewButton).toBeTruthy();
    expect(
      within(reviewButton)
        .getByRole("img", { name: "Repeater workstation" })
        .getAttribute("data-graph-semantic-icon"),
    )
      .toBe("repeater");
    expect(
      within(reviewButton)
        .getByRole("img", { name: "Active" })
        .getAttribute("data-graph-semantic-icon"),
    )
      .toBe("active-work");
    expect(
      (await getStateNodeArticle("story:documented"))
        .querySelector("article")
        ?.className.includes("border-af-overlay/22"),
    ).toBe(true);
    expect(screen.getByText("Active Story")).toBeTruthy();
    expect(screen.queryByText("dispatch-review-active")).toBeNull();
    expect(screen.queryByText("Active Work")).toBeNull();
    expect(screen.getByRole("button", { name: "Select story:blocked state" })).toBeTruthy();
  });

  it("renders every graph place family through custom React Flow node types", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(await screen.findByRole("region", { name: "Work graph viewport" })).toBeTruthy();
    await waitFor(() => {
      expect(document.querySelector("[data-current-activity-node-type='workstation']")).toBeTruthy();
    });

    expect(document.querySelector("[data-current-activity-node-type='statePosition']")).toBeTruthy();
    expect(document.querySelector("[data-current-activity-node-type='resource']")).toBeTruthy();
    expect(document.querySelector("[data-current-activity-node-type='constraint']")).toBeTruthy();
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
  });

  it("keeps zero-count resources visible and readable", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "agent-slot:available": 0 });
    renderCurrentActivity({ snapshot });

    const resourceCount = await screen.findByLabelText("0 resource tokens");
    const resourceNode = resourceCount.closest(".react-flow__node");
    const resourceArticle = resourceCount.closest("article");

    expect(resourceCount.textContent?.trim()).toBe("0");
    expect(screen.getByLabelText("agent-slot:available")).toBeTruthy();
    expect(resourceArticle?.querySelector("[data-place-work-type]")?.textContent).toBe("agent-slot");
    expect(resourceArticle?.querySelector("[data-place-state-value]")?.textContent).toBe("available");
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

  it("renders resource, constraint, and limit place role icons while preserving identifiers", async () => {
    renderCurrentActivity({ snapshot: dashboardSnapshotWithLimitPlace() });

    const resourceLabelContainer = await screen.findByLabelText("agent-slot:available");
    const resourceArticle = resourceLabelContainer.closest("article");
    const constraintArticle = screen.getByText("quality-gate:ready").closest("article");
    const limitArticle = screen.getByText("rate-limit:available").closest("article");

    expect(
      within(resourceArticle as HTMLElement)
        .getByRole("img", { name: "Resource" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(
      within(constraintArticle as HTMLElement)
        .getByRole("img", { name: "Constraint" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("constraint");
    expect(
      within(limitArticle as HTMLElement)
        .getByRole("img", { name: "Limit" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("limit");
    expect(resourceLabelContainer.getAttribute("aria-label")).toBe("agent-slot:available");
    expect(resourceArticle?.querySelector("[data-place-work-type]")?.textContent).toBe("agent-slot");
    expect(resourceArticle?.querySelector("[data-place-state-value]")?.textContent).toBe("available");
    expect(constraintArticle?.textContent).toContain("quality-gate:ready");
    expect(limitArticle?.textContent).toContain("rate-limit:available");
    expect(resourceArticle?.textContent).not.toContain("agent-slot:available");
    expect(resourceArticle?.textContent).not.toContain("Resource");
    expect(constraintArticle?.textContent).not.toContain("Constraint");
    expect(limitArticle?.textContent).not.toContain("Limit");
  });

  it("renders selected-tick resource counts while active dispatches occupy and return slots", async () => {
    const idleSnapshot = resourceOccupancySnapshotForTick(1);

    expect(idleSnapshot.runtime.in_flight_dispatch_count).toBe(0);
    expect(idleSnapshot.runtime.place_token_counts?.["agent-slot:available"]).toBe(2);

    renderCurrentActivity({ snapshot: idleSnapshot });

    const idleResourceCount = await screen.findByLabelText("2 resource tokens");

    expect(idleResourceCount.textContent?.trim()).toBe("2");
    expect(screen.getByLabelText("agent-slot:available")).toBeTruthy();

    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = installDashboardBrowserTestShims();

    const activeSnapshot = resourceOccupancySnapshotForTick(3);

    expect(activeSnapshot.runtime.in_flight_dispatch_count).toBe(1);
    expect(activeSnapshot.runtime.place_token_counts?.["agent-slot:available"]).toBe(1);

    renderCurrentActivity({ snapshot: activeSnapshot });

    const activeResourceCount = await screen.findByLabelText("1 resource tokens");

    expect(activeResourceCount.textContent?.trim()).toBe("1");
    expect(screen.getByLabelText("agent-slot:available")).toBeTruthy();
    expect(screen.queryByLabelText("2 resource tokens")).toBeNull();

    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = installDashboardBrowserTestShims();

    const returnedSnapshot = resourceOccupancySnapshotForTick(4);

    expect(returnedSnapshot.runtime.in_flight_dispatch_count).toBe(0);
    expect(returnedSnapshot.runtime.place_token_counts?.["agent-slot:available"]).toBe(2);

    renderCurrentActivity({ snapshot: returnedSnapshot });

    const returnedResourceCount = await screen.findByLabelText("2 resource tokens");

    expect(returnedResourceCount.textContent?.trim()).toBe("2");
    expect(screen.getByLabelText("agent-slot:available")).toBeTruthy();
    expect(screen.queryByLabelText("1 resource tokens")).toBeNull();
  });

  it("animates active graph flow while muting unrelated graph chrome", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const activeStateArticle = await getStateNodeArticle("story:complete");
    const idleStateArticle = await getStateNodeArticle("story:documented");

    await waitFor(() => {
      expect(document.querySelectorAll(".react-flow__edge-path").length).toBeGreaterThan(0);
    });

    const edgeStyles = Array.from(
      document.querySelectorAll<SVGPathElement>(".react-flow__edge-path"),
    ).map((edgePath) => edgePath.getAttribute("style") ?? "");
    const idleResourceArticle = screen.getByLabelText("agent-slot:available").closest("article");
    const activeEdges = document.querySelectorAll(".react-flow__edge.agent-flow-edge--active");

    expect(edgeStyles.some((style) => style.includes("var(--color-af-edge-muted"))).toBe(true);
    expect(edgeStyles.some((style) => style.includes("var(--color-af-success)"))).toBe(true);
    expect(edgeStyles.some((style) => style.includes("var(--color-af-accent)"))).toBe(false);
    expect(activeEdges.length).toBeGreaterThan(0);
    const activeEdgeLabels = Array.from(
      document.querySelectorAll<SVGTextElement>(".react-flow__edge.agent-flow-edge--active .react-flow__edge-text"),
    ).map((label) => label.textContent ?? "");

    expect(activeEdgeLabels.some((label) => label.length > 0)).toBe(true);
    expect(activeEdgeLabels.some((label) => label.includes("Flowing"))).toBe(false);
    expect(activeStateArticle.querySelector("article")?.className).toContain("border-af-success/70");
    expect(idleStateArticle.querySelector("article")?.className).toContain("opacity-[0.45]");
    expect(idleResourceArticle?.className).toContain("border-af-overlay/22");
    expect(idleResourceArticle?.className).not.toContain("opacity-[0.45]");
  });

  it("keeps inactive and failed output paths unlabeled and out of active green flow", async () => {
    renderCurrentActivity({ snapshot: dashboardSnapshotWithActiveImplementWorkstation() });

    await waitFor(() => {
      expect(document.querySelectorAll(".react-flow__edge-path").length).toBeGreaterThan(0);
    });

    const activeEdgeLabels = Array.from(
      document.querySelectorAll<SVGTextElement>(".react-flow__edge.agent-flow-edge--active .react-flow__edge-text"),
    ).map((label) => label.textContent ?? "");

    expect(activeEdgeLabels).toContain("story:implemented");
    expect(activeEdgeLabels).not.toContain("story:blocked");
    expect(document.querySelectorAll(".react-flow__edge.agent-flow-edge--active")).toHaveLength(2);
    expect(screen.queryByText(/Flowing/)).toBeNull();
    expect(screen.queryByText(/Failure Path/)).toBeNull();
    expect(
      document.querySelectorAll(".react-flow__edge.agent-flow-edge--active.agent-flow-edge--semantic"),
    ).toHaveLength(0);
  });

  it("hides workstation return edges to resource nodes while keeping resource inputs visible", async () => {
    renderCurrentActivity({ snapshot: dashboardSnapshotWithResourceReturnEdge() });

    await waitFor(() => {
      expect(document.querySelectorAll(".react-flow__edge-path").length).toBeGreaterThan(0);
    });

    expect(
      document.querySelector(
        '[data-id="place:agent-slot:available:workstation:implement:input"]',
      ),
    ).toBeTruthy();
    expect(
      document.querySelector(
        '[data-id="workstation:implement:place:agent-slot:available:output"]',
      ),
    ).toBeNull();
  });

  it("uses selected accent styling over active flow styling", async () => {
    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      selection: { kind: "state-node", placeId: "story:complete" },
    });

    const activeSelectedState = await getStateNodeArticle("story:complete");
    const activeSelectedArticle = activeSelectedState.querySelector("article");

    expect(activeSelectedArticle?.className).toContain("border-af-accent/70");
    expect(activeSelectedArticle?.className).not.toContain("border-af-success/70");

    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = installDashboardBrowserTestShims();

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      selection: { kind: "node", nodeId: "review" },
    });

    const reviewButton = await screen.findByRole("button", { name: "Select Review workstation" });
    const reviewArticle = reviewButton.closest("article");

    expect(reviewArticle?.className).toContain("border-af-accent/70");
    expect(reviewArticle?.className).not.toContain("agent-flow-node--active");
  });

  it("renders the legend minimized by default and expands it for graph node and edge semantics", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const expandButton = await screen.findByRole("button", { name: "Expand graph legend" });

    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText("Graph legend")).toBeNull();

    const legend = await expandGraphLegend();
    const legendScope = within(legend);
    const collapseButton = screen.getByRole("button", { name: "Collapse graph legend" });

    expect(legendScope.getByText("Active flow")).toBeTruthy();
    expect(legendScope.getByText("Failure path")).toBeTruthy();
    expect(legend.querySelector("[data-legend-flow]")).toBeTruthy();
    expect(collapseButton.getAttribute("aria-expanded")).toBe("true");
    for (const [label, kind] of LEGEND_ICON_EXPECTATIONS) {
      const icon = legendScope.getByRole("img", { name: `${label} legend icon` });

      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(kind);
      expect(legendScope.getByText(label)).toBeTruthy();
      expect(legend.querySelector(`[data-legend-icon='${kind}']`)).toBeTruthy();
    }
    expect(legend.querySelector("[data-legend-icon='queue'] span.h-3")).toBeNull();
    expect(legend.querySelector("[data-legend-icon='workstation'] span.border-2")).toBeNull();
    expect(legend.querySelector("[data-legend-icon='exhaustion'] span.border-dashed")).toBeNull();

    fireEvent.click(collapseButton);

    await waitFor(() => {
      expect(screen.queryByLabelText("Graph legend")).toBeNull();
    });
    expect(screen.getByRole("button", { name: "Expand graph legend" })).toBeTruthy();
  });

  it("renders exhaustion-rule transitions as compact non-work nodes", async () => {
    renderCurrentActivity({ snapshot: dashboardSnapshotWithExhaustionRuleNode() });

    const exhaustionButton = await screen.findByRole("button", {
      name: "Select executor-loop-breaker exhaustion rule",
    });
    const exhaustionNode = exhaustionButton.closest(".react-flow__node");
    const exhaustionArticle = exhaustionButton.closest("article");

    expect(exhaustionNode?.getAttribute("style")).toContain("width: 132px");
    expect(exhaustionNode?.getAttribute("style")).toContain("height: 58px");
    expect(exhaustionArticle?.className).toContain("border-dashed");
    expect(
      within(exhaustionButton)
        .getByRole("img", { name: "Exhaustion rule" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("exhaustion");
    expect(exhaustionButton.textContent).not.toContain("Exhaustion");
    expect(screen.queryByRole("button", { name: /Should Not Render/ })).toBeNull();
    expect(screen.queryByText("Should Not Render")).toBeNull();
  });

  it("renders the shared workstation-kind parity fixture with distinct supported icons", async () => {
    const { onSelectWorkstation } = renderCurrentActivity({
      snapshot: workstationKindParityDashboardSnapshot,
    });

    const legend = await expandGraphLegend();
    const legendScope = within(legend);

    for (const expectation of workstationKindParityExpectations) {
      const button = await screen.findByRole("button", { name: expectation.buttonName });
      const icon = within(button).getByRole("img", { name: expectation.metadata.label });
      const legendIcon = legendScope.getByRole("img", {
        name: `${expectation.metadata.label} legend icon`,
      });

      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(expectation.metadata.iconKind);
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
        .getByRole("img", { name: "Processing state" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("processing");
    expect(
      within(terminalStateArticle)
        .getByRole("img", { name: "Terminal" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("terminal");
    expect(
      within(failedStateArticle)
        .getByRole("img", { name: "Failed" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("failed");
    expect(initialStateArticle.querySelector("[data-state-work-type]")?.textContent).toBe("story");
    expect(initialStateArticle.querySelector("[data-state-value]")?.textContent).toBe("init");
    expect(processingStateArticle.querySelector("[data-state-value]")?.textContent).toBe("ready");
    expect(terminalStateArticle.querySelector("[data-state-value]")?.textContent).toBe("complete");
    expect(failedStateArticle.querySelector("[data-state-value]")?.textContent).toBe("blocked");
    expect(initialStateArticle.querySelector("article")?.textContent).not.toContain("Queue");
    expect(terminalStateArticle.querySelector("article")?.textContent).not.toContain("Terminal");
    expect(failedStateArticle.querySelector("article")?.textContent).not.toContain("Failed");
    expect(failedStateArticle.querySelector("article")?.textContent).not.toContain("Queue");
    expect(failedStateArticle.querySelector("article")?.className).toContain(
      "border-af-edge-danger-muted",
    );
  });

  it("themes the React Flow controls with dashboard colors", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(await screen.findByRole("region", { name: "Work graph viewport" })).toBeTruthy();
    const controls = document.querySelector<HTMLElement>(".react-flow__controls");
    const zoomIn = controls?.querySelector<HTMLButtonElement>(".react-flow__controls-zoomin");

    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-button-background-color-props: rgb(from var(--color-af-surface) r g b / 0.94)",
    );
    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-button-color-props: rgb(from var(--color-af-ink) r g b / 0.72)",
    );
    expect(controls?.getAttribute("style")).toContain("--xy-controls-box-shadow: none");
    expect(controls?.getAttribute("style")).not.toContain("#fefefe");
    expect(zoomIn).toBeTruthy();
  });

  it("renders state-position markers as green dots for low-count active states", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 3 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const dotContainer = readyStateArticle.querySelector("[data-state-work-progress='dots']");
    const dots = readyStateArticle.querySelectorAll("[data-state-work-progress-dot]");
    const dotIndices = Array.from(dots).map((dot) => dot.getAttribute("data-state-work-progress-dot"));

    expect(dotContainer).toBeTruthy();
    expect(dotIndices).toEqual(["0", "1", "2"]);
    expect(dotContainer?.getAttribute("aria-label")).toBe("3 active items");
    expect(dotContainer?.querySelector?.("span")).not.toBeNull();
  });

  it("renders work-state labels and markers in separated stable zones", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 3 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const labelZone = readyStateArticle.querySelector("[data-state-label-zone]");
    const markerZone = readyStateArticle.querySelector("[data-state-marker-zone]");
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
    expect(markerZone?.querySelectorAll("[data-state-work-progress-dot]")).toHaveLength(3);
  });

  it("renders exactly 10 state-position markers in a compact ordered grid", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 10 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const dotContainer = readyStateArticle.querySelector("[data-state-work-progress='dots']");
    const dotIndices = Array.from(
      readyStateArticle.querySelectorAll("[data-state-work-progress-dot]"),
    ).map((dot) => dot.getAttribute("data-state-work-progress-dot"));

    expect(dotContainer?.className).toContain("grid-cols-[repeat(5,0.5rem)]");
    expect(dotIndices).toEqual(["0", "1", "2", "3", "4", "5", "6", "7", "8", "9"]);
  });

  it("uses numeric fallback for state-position active counts above 10", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 11 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const numeric = readyStateArticle.querySelector("[data-state-work-progress='numeric']");

    expect(numeric).toBeTruthy();
    expect(numeric?.textContent?.trim()).toBe("11");
    expect(numeric?.getAttribute("aria-label")).toBe("11 active items");
    expect(readyStateArticle.querySelector("[data-state-work-progress='dots']")).toBeNull();
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
    expect(longStateButton.className).toContain("grid-rows-[1.5rem_auto]");
    expect(longStateButton.className).toContain("overflow-hidden");
    expect(labelZone?.className).toContain("h-[1.5rem]");
    expect(labelZone?.className).toContain("max-h-[1.5rem]");
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
    const documentedStateArticle = await getStateNodeArticle("story:documented");

    expect(readyStateArticle.querySelector("article")?.className).toContain("border-af-overlay/22");
    expect(documentedStateArticle.querySelector("article")?.className).toContain("border-af-overlay/22");
  });

  it("selects workstation and work item context through the dashboard callbacks", async () => {
    const { onSelectWorkItem, onSelectWorkstation } = renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(await screen.findByRole("button", { name: "Select Review workstation" }));

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    await waitFor(() => {
      expect(onSelectWorkItem).toHaveBeenCalled();
    });
    expect(onSelectWorkItem.mock.calls[0]?.[0]).toBe("dispatch-review-active");
    expect(onSelectWorkItem.mock.calls[0]?.[1]).toBe("review");
    expect(onSelectWorkItem.mock.calls[0]?.[3].work_id).toBe("work-active-story");
  });

  it("caps workstation work item names at three and summarizes the rest", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(5),
    });

    expect(await screen.findByRole("button", { name: /Active Story 1/ })).toBeTruthy();
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

    const reviewButton = await screen.findByRole("button", { name: "Select Review workstation" });
    const reviewNode = reviewButton.closest(".react-flow__node");
    const dots = reviewNode?.querySelector("[data-workstation-work-progress='dots']");

    expect(dots).toBeTruthy();
    expect(dots?.getAttribute("aria-label")).toBe("6 active items");
    expectFixedWorkstationNodeDimensions(reviewNode);
    expect(screen.getByRole("button", { name: /Active Story 1/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Active Story 3/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Active Story 4/ })).toBeNull();
    expect(reviewNode?.querySelector("article")?.className).toContain("border-af-success/50");

    fireEvent.click(reviewButton);

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");
  });

  it("keeps workstation node dimensions fixed across zero, one, five, and six active items", async () => {
    const { rerender } = renderWithQueryClient(
      <ReactFlowCurrentActivityCard
        now={Date.parse("2026-04-08T12:00:04Z")}
        selection={null}
        snapshot={dashboardSnapshotWithActiveWorkItemCount(0)}
        onSelectWorkItem={vi.fn()}
        onSelectStateNode={vi.fn()}
        onSelectWorkstation={vi.fn()}
      />,
    );

    for (const activeItemCount of [0, 1, 5, 6]) {
      rerender(
        <QueryClientProvider client={new QueryClient({
          defaultOptions: {
            mutations: { retry: false },
            queries: { gcTime: Infinity, retry: false },
          },
        })}
        >
          <ReactFlowCurrentActivityCard
            now={Date.parse("2026-04-08T12:00:04Z")}
            selection={null}
            snapshot={dashboardSnapshotWithActiveWorkItemCount(activeItemCount)}
            onSelectWorkItem={vi.fn()}
            onSelectStateNode={vi.fn()}
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
    const zeroActiveLayout = await buildGraphLayout(zeroActiveSnapshot.topology);
    const sixActiveLayout = await buildGraphLayout(sixActiveSnapshot.topology);
    const graphKey = currentActivityGraphKey(zeroActiveLayout);

    expect(currentActivityGraphKey(sixActiveLayout)).toBe(graphKey);

    useCurrentActivityGraphStore
      .getState()
      .setNodePosition(graphKey, "workstation:review", { x: 321, y: 654 });

    const callbacks = {
      onSelectStateNode: vi.fn(),
      onSelectWorkItem: vi.fn(),
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
    await waitFor(() => {
      expect(reviewNode.getAttribute("style")).toContain("translate(321px,654px)");
    });
    expectFixedWorkstationNodeDimensions(reviewNode);

    rerender(
      <QueryClientProvider client={new QueryClient({
        defaultOptions: {
          mutations: { retry: false },
          queries: { gcTime: Infinity, retry: false },
        },
      })}
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
    await waitFor(() => {
      expect(reviewNode.getAttribute("style")).toContain("translate(321px,654px)");
    });
    expectFixedWorkstationNodeDimensions(reviewNode);
  });

  it("derives a stable topology cache key for equivalent cloned workflow topology", () => {
    const firstKey = currentActivityTopologyKey(semanticWorkflowDashboardSnapshot.topology);
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
    expect(screen.queryByRole("button", { name: "Select agent-slot:available state" })).toBeNull();

    fireEvent.click(await screen.findByRole("button", { name: "Select story:ready state" }));

    expect(onSelectStateNode).toHaveBeenCalledWith("story:ready");
  });

  it("keeps long workstation and active work labels from hiding the duration", async () => {
    const labels = [
      "Short Active Story",
      "Active Story With A Medium Sized Label",
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    ];
    const { onSelectWorkItem } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(),
    });
    const longWorkstationButton = await screen.findByRole("button", {
      name: /Select Review Requests With A Deliberately Long Workstation Title workstation/,
    });
    const longWorkstationLabel = longWorkstationButton.querySelector("[data-workstation-title]");
    const longWorkButton = await screen.findByRole("button", {
      name: /Active Story With A Deliberately Long Label/,
    });
    const longWorkLabel = longWorkButton.querySelector("[data-active-work-label]");
    const durationLabel = longWorkButton.querySelector("[data-active-work-duration]");
    const reviewNode = longWorkButton.closest(".react-flow__node");

    expect(reviewNode?.getAttribute("style")).toContain("width: 156px");
    expect(longWorkstationButton.getAttribute("title")).toBe(
      "Review Requests With A Deliberately Long Workstation Title",
    );
    expect(longWorkstationLabel?.className).toContain("truncate");
    expect(longWorkstationLabel?.className).toContain("whitespace-nowrap");
    expect(longWorkButton.className).toContain("min-w-0");
    expect(longWorkButton.className).toContain("grid-cols-[minmax(0,1fr)_auto]");
    expect(longWorkButton.className).toContain("overflow-hidden");
    expect(longWorkLabel?.className).toContain("truncate");
    expect(longWorkLabel?.className).toContain("basis-0");
    expect(durationLabel?.textContent).toBe("4s");
    expect(durationLabel?.className).toContain("whitespace-nowrap");
    expect(durationLabel?.className).toContain("text-right");
    expect(durationLabel?.className).not.toContain("overflow-hidden");
    labels.forEach((label) => {
      const activeWorkButton = screen.getByRole("button", { name: new RegExp(label) });
      const labelElement = activeWorkButton.querySelector("[data-active-work-label]");
      const durationElement = activeWorkButton.querySelector("[data-active-work-duration]");

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
      expect(onSelectWorkItem).toHaveBeenCalled();
    });
    expect(onSelectWorkItem.mock.calls[0]?.[3].display_name).toBe(
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    );

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
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];

    if (!activeExecution) {
      throw new Error("expected semantic workflow fixture to include an active review execution");
    }

    activeExecution.work_items = [
      {
        trace_id: "trace-malformed-active-story",
        work_type_id: "story",
      } as DashboardWorkItemRef,
    ];
    activeExecution.trace_ids = ["trace-malformed-active-story"];

    renderCurrentActivity({ snapshot });

    expect(await screen.findByRole("button", { name: "Select Review workstation" })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Unknown work/ })).toBeTruthy();
  });

  it("renders a single-node workflow without edge data", async () => {
    renderCurrentActivity({ snapshot: singleNodeDashboardSnapshot });

    expect(await screen.findByRole("button", { name: "Select Intake workstation" })).toBeTruthy();
    expect(screen.queryByText("Idle")).toBeNull();
  });

  it("renders a twenty-node workflow fixture for larger graphs", async () => {
    renderCurrentActivity({ snapshot: twentyNodeDashboardSnapshot });

    expect(await screen.findByRole("button", { name: "Select Station 20 workstation" })).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /Select .* workstation/ })).toHaveLength(20);
    expect(screen.getAllByRole("img", { name: "Standard workstation" }).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByRole("img", { name: "Queue" }).getAttribute("data-graph-semantic-icon"))
      .toBe("queue");
    const legend = await expandGraphLegend();
    expect(
      within(screen.getByLabelText("Graph legend"))
        .getByRole("img", { name: "Standard workstation legend icon" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("workstation");
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.getByRole("button", { name: "Select story:step-6 state" })).toBeTruthy();
  });

  it("uses persisted graph node positions when the topology remounts", async () => {
    const layout = await buildGraphLayout(semanticWorkflowDashboardSnapshot.topology);
    const graphKey = currentActivityGraphKey(layout);

    useCurrentActivityGraphStore
      .getState()
      .setNodePosition(graphKey, "workstation:review", { x: 777, y: 333 });
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const reviewButton = await screen.findByRole("button", { name: "Select Review workstation" });
    const reviewNode = reviewButton.closest(".react-flow__node");

    await waitFor(() => {
      expect(reviewNode?.getAttribute("style")).toContain("translate(777px,333px)");
    });
  });
});
