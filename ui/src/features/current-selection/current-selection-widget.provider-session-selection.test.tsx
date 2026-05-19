import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import {
  buildDashboardInferenceAttemptFixture,
  buildDashboardWorkstationRequestFixture,
} from "../../components/dashboard/fixtures";
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

function buildSelectedWorkItemFixture() {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const dispatchId = snapshot.runtime.active_dispatch_ids?.[0] ?? "";
  const execution =
    snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId];
  const workItem = execution?.work_items?.[0];
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

  if (!execution || !workItem || !selectedNode) {
    throw new Error(
      "expected semantic workflow fixture to include an active selected work item",
    );
  }

  const selection: DashboardSelection = {
    dispatchId,
    execution,
    kind: "work-item",
    nodeId: selectedNode.node_id,
    workItem,
  };

  return {
    dispatchId,
    execution,
    selectedNode,
    selection,
    snapshot,
    workItem,
  };
}

describe("CurrentSelectionWidget provider-session selection", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    vi.stubGlobal("fetch", vi.fn());
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue({
      data: undefined,
      error: null,
      failureCount: 0,
      failureReason: null,
      fetchStatus: "idle",
      isError: false,
      isFetched: false,
      isFetchedAfterMount: false,
      isFetching: false,
      isInitialLoading: false,
      isLoading: false,
      isLoadingError: false,
      isPaused: false,
      isPending: true,
      isPlaceholderData: false,
      isRefetchError: false,
      isRefetching: false,
      isStale: true,
      isSuccess: false,
      promise: Promise.resolve(undefined),
      refetch: vi.fn(),
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

  it("keeps an inference-attempt-only provider session selected in current-selection state", async () => {
    const user = userEvent.setup();
    const { dispatchId, execution, selectedNode, selection, snapshot, workItem } =
      buildSelectedWorkItemFixture();
    const inferenceOnlyRequest = buildDashboardWorkstationRequestFixture(dispatchId, {
      inference_attempts: [
        buildDashboardInferenceAttemptFixture(dispatchId, {
          outcome: "SUCCEEDED",
          provider_session: {
            id: "sess_inference_only",
            kind: "session_id",
            provider: "codex",
          },
          response: "Selected from inference attempts.",
        }),
      ],
    });
    const executionDetails = selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID: snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions: [],
      selectedNode,
      workItem,
    });

    vi.mocked(globalThis.fetch).mockImplementation((input) => {
      const requestURL = String(input);
      if (!requestURL.includes("id=sess_inference_only")) {
        throw new Error(`unexpected provider-session request: ${requestURL}`);
      }

      return Promise.resolve(
        new Response(
          JSON.stringify({
            parse: {
              eventCount: 1,
              functionCalls: [],
              lineCount: 1,
              malformedLineCount: 0,
              parseErrors: [],
              reasoning: [],
              turns: [],
              unknownEventCount: 0,
              unknownEvents: [],
            },
            providerSession: {
              id: "sess_inference_only",
              kind: "session_id",
              provider: "codex",
            },
            source: {
              relativePath: "2026/05/18/rollout-sess_inference_only.jsonl",
              sizeBytes: 640,
            },
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
            statusText: "OK",
          },
        ),
      );
    });

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedWorkDispatchAttempts: [],
          selectedWorkProviderSessions: [],
          selectedWorkRequestHistory: [inferenceOnlyRequest],
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const selectSessionButton = within(currentSelection).getByRole("button", {
      name: "Select provider session codex / session_id / sess_inference_only for dispatch dispatch-review-active",
    });

    expect(selectSessionButton.getAttribute("aria-pressed")).toBe("false");

    await user.click(selectSessionButton);

    expect(
      within(currentSelection)
        .getByRole("button", {
          name: "Select provider session codex / session_id / sess_inference_only for dispatch dispatch-review-active",
        })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Selected session details",
      }),
    ).toBeTruthy();
    expect(
      await within(currentSelection).findByText(
        "2026/05/18/rollout-sess_inference_only.jsonl",
      ),
    ).toBeTruthy();
  });
});

function renderWithQueryClient(view: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}
