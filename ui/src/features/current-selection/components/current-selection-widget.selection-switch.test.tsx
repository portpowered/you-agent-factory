import { screen, within } from "@testing-library/react";
import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { CurrentSelectionWidget } from "./current-selection-widget";
import {
  createCurrentSelectionWidgetQueryClient,
  renderWithExistingQueryClient,
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

describe("CurrentSelectionWidget selection switching", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
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
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
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
  });

  it("replaces workstation configuration with the selected non-workstation detail view", () => {
    const queryClient = createCurrentSelectionWidgetQueryClient();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const dispatchId = snapshot.runtime.active_dispatch_ids?.[0] ?? "";
    const execution =
      snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId];
    const workItem = execution?.work_items?.[0];
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
      throw new Error("expected current-selection work-item fixture");
    }

    const selection: DashboardSelection = {
      dispatchId,
      execution,
      kind: "work-item",
      nodeId: selectedNode.node_id,
      workItem,
    };
    const executionDetails = selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID:
        snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions: providerSessions ?? [],
      selectedNode,
      workItem,
      workstationRequestsByDispatchID:
        snapshot.runtime.workstation_requests_by_dispatch_id,
    });

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(screen.getByRole("heading", { name: "Configuration" })).toBeTruthy();

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions ?? [],
          selectedWorkDispatchAttempts: providerSessions ?? [],
          selectedWorkProviderSessions: providerSessions ?? [],
          selectedWorkRequestHistory,
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Configuration",
      }),
    ).toBeNull();
    expect(within(currentSelection).getByText(workItem.work_id)).toBeTruthy();
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
  });
});

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
    selectedWorkWorkstationRequests: [],
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

function buildEditableDefinitionResult(
  data: CanonicalFactoryDefinition | undefined,
) {
  return {
    data,
    error: null,
    failureCount: 0,
    failureReason: null,
    fetchStatus: "idle",
    isError: false,
    isFetched: true,
    isFetchedAfterMount: true,
    isFetching: false,
    isInitialLoading: false,
    isLoading: false,
    isLoadingError: false,
    isPaused: false,
    isPending: false,
    isPlaceholderData: false,
    isRefetchError: false,
    isRefetching: false,
    isStale: true,
    isSuccess: true,
    promise: Promise.resolve(data),
    refetch: vi.fn(),
    status: "success",
  } as never;
}

function buildEditableFactoryDefinition(): CanonicalFactoryDefinition {
  return {
    name: "Current Factory",
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.6",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
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
}

