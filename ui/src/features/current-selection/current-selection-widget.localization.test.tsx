import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { useCurrentEditableFactoryDefinition } from "../current-factory-definition";
import { CurrentSelectionWidget } from "./current-selection-widget";
import { selectWorkItemExecutionDetails } from "./state/executionDetails";
import { resetSelectionHistoryStore } from "./state/selectionHistoryStore";
import type { DashboardSelection } from "./types";
import { useSaveEditableWorkstationConfiguration } from "./use-save-editable-workstation-configuration";
import type { CurrentSelectionState } from "./useCurrentSelection";

vi.mock("../current-factory-definition", async () => {
  const actual = await vi.importActual("../current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinition: vi.fn(),
  };
});

vi.mock("./use-save-editable-workstation-configuration", () => ({
  useSaveEditableWorkstationConfiguration: vi.fn(),
}));

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

function buildCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  return {
    canRedoSelection: false,
    canUndoSelection: false,
    completedWorkItems: [],
    failedWorkItems: [],
    openTerminalWorkDetail: () => undefined,
    redoSelection: () => undefined,
    selectedNode: null,
    selectedNodeActiveExecutions: [],
    selectedNodeProviderSessions: [],
    selectedNodeWorkstationRequests: [],
    selectedStateCurrentWorkItems: [],
    selectedStatePlace: null,
    selectedStateTerminalHistoryWorkItems: [],
    selectedStateTokenCount: 0,
    selectedWorkDispatchAttempts: [],
    selectedWorkID: null,
    selectedWorkProviderSessions: [],
    selectedWorkRequestHistory: [],
    selectedWorkstationRequest: null,
    selection: null,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkByID: () => undefined,
    selectWorkItem: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

function buildSelectedWorkItemFixture() {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const dispatchId = snapshot.runtime.active_dispatch_ids?.[0] ?? "";
  const execution =
    snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId];
  const workItem = execution?.work_items?.[0];
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
  const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
    (attempt) =>
      attempt.work_items?.some(
        (candidate) => candidate.work_id === workItem?.work_id,
      ),
  );
  const selectedWorkRequestHistory = snapshot.runtime
    .workstation_requests_by_dispatch_id?.[dispatchId]
    ? [snapshot.runtime.workstation_requests_by_dispatch_id[dispatchId]]
    : [];

  if (!execution || !workItem || !selectedNode) {
    throw new Error("expected semantic workflow selected work fixture");
  }

  const selection: DashboardSelection = {
    dispatchId,
    execution,
    kind: "work-item",
    nodeId: selectedNode.node_id,
    workItem,
  };

  return {
    executionDetails: selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID:
        snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions: providerSessions ?? [],
      selectedNode,
      workItem,
      workstationRequestsByDispatchID:
        snapshot.runtime.workstation_requests_by_dispatch_id,
    }),
    providerSessions: providerSessions ?? [],
    selectedNode,
    selectedWorkRequestHistory,
    selection,
    workItem,
  };
}

describe("CurrentSelectionWidget localization", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    vi.stubGlobal("fetch", vi.fn());
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue({
      data: undefined,
      isPending: true,
      status: "pending",
    } as never);
    vi.mocked(useSaveEditableWorkstationConfiguration).mockReturnValue({
      beginSaveConfirmation: vi.fn(),
      canSave: false,
      cancelSaveConfirmation: vi.fn(),
      confirmSave: vi.fn(),
      saveState: { status: "idle" },
    });
  });

  afterEach(() => {
    resetSelectionHistoryStore();
    vi.unstubAllGlobals();
  });

  it("switches shell, dispatch, and provider-session copy to zh-CN while preserving data values", async () => {
    const user = userEvent.setup();
    const fixture = buildSelectedWorkItemFixture();
    const currentSelection = buildCurrentSelection({
      selectedNode: fixture.selectedNode,
      selectedNodeProviderSessions: fixture.providerSessions,
      selectedWorkDispatchAttempts: fixture.providerSessions,
      selectedWorkProviderSessions: fixture.providerSessions,
      selectedWorkRequestHistory: fixture.selectedWorkRequestHistory,
      selection: fixture.selection,
    });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    vi.mocked(globalThis.fetch).mockReturnValue(
      new Promise<Response>(() => undefined),
    );

    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <CurrentSelectionWidget
          currentSelection={currentSelection}
          locale="en"
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={fixture.executionDetails}
        />
      </QueryClientProvider>,
    );

    const englishSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(englishSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(within(englishSelection).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(englishSelection).getByText(fixture.workItem.work_id),
    ).toBeTruthy();

    await user.click(
      within(englishSelection).getByRole("button", {
        name: "Select provider session codex / session_id / sess-active-story for dispatch dispatch-review-active",
      }),
    );
    expect(
      await within(englishSelection).findByText("Loading session details..."),
    ).toBeTruthy();

    rerender(
      <QueryClientProvider client={queryClient}>
        <CurrentSelectionWidget
          currentSelection={currentSelection}
          locale="zh-CN"
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={fixture.executionDetails}
        />
      </QueryClientProvider>,
    );

    const localizedSelection = screen.getByRole("article", {
      name: "当前选择",
    });
    expect(
      within(localizedSelection).getByRole("heading", { name: "工作站分派" }),
    ).toBeTruthy();
    expect(within(localizedSelection).getByText("当前分派")).toBeTruthy();
    expect(
      within(localizedSelection).getByRole("button", {
        name: "选择调度 dispatch-review-active 的 provider session codex / session_id / sess-active-story",
      }),
    ).toBeTruthy();
    expect(
      within(localizedSelection).getByText("正在加载会话详情..."),
    ).toBeTruthy();
    expect(
      within(localizedSelection).getByText(fixture.workItem.work_id),
    ).toBeTruthy();
    expect(
      within(localizedSelection).getByText("dispatch-review-active"),
    ).toBeTruthy();
    expect(
      within(localizedSelection).getAllByText(/sess-active-story/).length,
    ).toBeGreaterThan(0);
    expect(within(localizedSelection).getByText("Active Story")).toBeTruthy();
  });
});
