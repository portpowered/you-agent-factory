import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  useCurrentFactoryDefinition,
  useCurrentFactoryDocument,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { CurrentSelectionWidget } from "./current-selection-widget";
import {
  createCurrentSelectionWidgetQueryClient,
  wrapCurrentSelectionWidgetView,
} from "./current-selection-widget-test-utils";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";
import { resetSelectionHistoryStore } from "../state/selectionHistoryStore";
import type { DashboardSelection } from "../state/selection-types";
import { useSaveEditableWorkstationConfiguration } from "../workstation-selection/hooks/use-save-editable-workstation-configuration";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { useCurrentWorkstationPromptTemplateValidation } from "../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation";

vi.mock("../../current-factory-definition/hooks/useCurrentFactoryDefinition", async () => {
  const actual = await vi.importActual(
    "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  );

  return {
    ...actual,
    useCurrentFactoryDefinition: vi.fn(),
    useCurrentFactoryDocument: vi.fn(),
  };
});

vi.mock("../workstation-selection/hooks/use-save-editable-workstation-configuration", () => ({
  useSaveEditableWorkstationConfiguration: vi.fn(),
}));

vi.mock("../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation", () => ({
  useCurrentWorkstationPromptTemplateValidation: vi.fn(),
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

function setupCurrentSelectionLocalizationTest() {
  beforeEach(() => {
    resetSelectionHistoryStore();
    vi.stubGlobal("fetch", vi.fn());
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockReturnValue({
      data: {
        diagnostics: [],
        valid: true,
      },
      error: null,
      isError: false,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);
    vi.mocked(useCurrentFactoryDefinition).mockReturnValue({
      data: undefined,
      isPending: true,
      status: "pending",
    } as never);
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
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
}

describe("CurrentSelectionWidget localization", () => {
  setupCurrentSelectionLocalizationTest();

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
    const queryClient = createCurrentSelectionWidgetQueryClient();

    vi.mocked(globalThis.fetch).mockReturnValue(
      new Promise<Response>(() => undefined),
    );

    const { rerender } = render(
      wrapCurrentSelectionWidgetView(queryClient, <CurrentSelectionWidget
          currentSelection={currentSelection}
          locale="en"
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={fixture.executionDetails}
        />),
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
        name: "Select provider session codex / Session ID / sess-active-story for dispatch dispatch-review-active",
      }),
    );
    expect(
      within(englishSelection).getByText("Session selected"),
    ).toBeTruthy();

    rerender(
      wrapCurrentSelectionWidgetView(queryClient, <CurrentSelectionWidget
          currentSelection={currentSelection}
          locale="zh-CN"
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={fixture.executionDetails}
        />),
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
        name: "选择调度 dispatch-review-active 的 provider session codex / 会话 ID / sess-active-story",
      }),
    ).toBeTruthy();
    expect(
      within(localizedSelection).getByText("会话已选中"),
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

describe("CurrentSelectionWidget workstation localization", () => {
  setupCurrentSelectionLocalizationTest();

  it("switches workstation kind labels to zh-CN without changing canonical values", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = {
      ...snapshot.topology.workstation_nodes_by_id.review,
      workstation_kind: "repeater",
    };
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const factoryDocument: CurrentFactoryDocument = {
      name: "Current Factory",
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        { model: "gpt-5.5", name: "reviewer", type: "MODEL_WORKER" },
        { model: "gpt-5.6", name: "planner", type: "MODEL_WORKER" },
      ],
      workstations: [
        {
          behavior: "STANDARD",
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/review.md",
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    vi.mocked(useCurrentFactoryDefinition).mockReturnValue({
      data: factoryDocument,
      isPending: false,
      status: "success",
    } as never);
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: factoryDocument,
      error: null,
      isError: false,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    const currentSelection = buildCurrentSelection({
      selectedNode,
      selection: { kind: "node", nodeId: selectedNode.node_id },
    });

    const { rerender } = render(
      wrapCurrentSelectionWidgetView(queryClient, <CurrentSelectionWidget
          currentSelection={currentSelection}
          locale="en"
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />),
    );

    expect(screen.getByRole("heading", { name: "Workstation summary" })).toBeTruthy();
    expect(screen.getByText("Standard")).toBeTruthy();
    expect(screen.getByText("reviewer")).toBeTruthy();

    rerender(
      wrapCurrentSelectionWidgetView(queryClient, <CurrentSelectionWidget
          currentSelection={currentSelection}
          locale="zh-CN"
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />),
    );

    expect(screen.getByRole("heading", { name: "工作站摘要" })).toBeTruthy();
    expect(screen.getByText("标准")).toBeTruthy();
    expect(screen.getByText("reviewer")).toBeTruthy();
  });
});
