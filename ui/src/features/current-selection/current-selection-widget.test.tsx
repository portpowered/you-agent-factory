import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import {
  buildDashboardInferenceAttemptFixture,
  buildDashboardWorkstationRequestFixture,
} from "../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { useCurrentEditableFactoryDefinition } from "../current-factory-definition";
import { CurrentSelectionWidget } from "./current-selection-widget";
import { selectWorkItemExecutionDetails } from "./state/executionDetails";
import { resetSelectionHistoryStore } from "./state/selectionHistoryStore";
import type { DashboardSelection, TerminalWorkDetail } from "./types";
import { useSaveEditableWorkstationConfiguration } from "./use-save-editable-workstation-configuration";
import type { CurrentSelectionState } from "./useCurrentSelection";
import { useCurrentWorkstationPromptTemplateValidation } from "./useCurrentWorkstationPromptTemplateValidation";

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

vi.mock("./useCurrentWorkstationPromptTemplateValidation", () => ({
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
    selectedNodeWorkstationRequests: [],
    selection: null,
    selectWorkByID: () => undefined,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
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
    selectedWorkRequestHistory,
    selectedNode,
    selection,
    snapshot,
    workItem,
  };
}

describe("CurrentSelectionWidget", () => {
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

  it("keeps the work item card visible even when terminal work metadata is present", () => {
    const {
      executionDetails,
      providerSessions,
      selectedNode,
      selectedWorkRequestHistory,
      selection,
      workItem,
    } = buildSelectedWorkItemFixture();
    const terminalWorkDetail: TerminalWorkDetail = {
      attempts: providerSessions,
      label: workItem.display_name ?? workItem.work_id,
      status: "failed",
      traceWorkID: workItem.work_id,
    };

    vi.mocked(globalThis.fetch).mockResolvedValue(
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
            id: "sess_active",
            kind: "session_id",
            provider: "codex",
          },
          source: {
            relativePath: "2026/05/18/rollout-sess_active.jsonl",
            sizeBytes: 512,
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

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions,
          selectedWorkProviderSessions: providerSessions,
          selectedWorkRequestHistory,
          selectedWorkDispatchAttempts: providerSessions,
          selection,
          terminalWorkDetail,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(within(currentSelection).getByText(workItem.work_id)).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(within(currentSelection).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Work session runs list",
      }),
    ).toBeNull();
  });

  it("renders work item details when the active selection is a work item", () => {
    const {
      executionDetails,
      providerSessions,
      selectedNode,
      selectedWorkRequestHistory,
      selection,
      workItem,
    } = buildSelectedWorkItemFixture();

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions,
          selectedWorkProviderSessions: providerSessions,
          selectedWorkRequestHistory,
          selectedWorkDispatchAttempts: providerSessions,
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(within(currentSelection).getByText(workItem.work_id)).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(within(currentSelection).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Work session runs list",
      }),
    ).toBeNull();
  });

  it("selects a provider-session card without changing the selected work item detail", async () => {
    const user = userEvent.setup();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const dispatchId = "dispatch-review-active";
    const execution =
      snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId];
    const workItem = execution?.work_items?.[0];

    if (!execution || !workItem || !selectedNode) {
      throw new Error(
        "expected current-selection fixture for provider-session selection",
      );
    }

    const selection: DashboardSelection = {
      dispatchId,
      execution,
      kind: "work-item",
      nodeId: selectedNode.node_id,
      workItem,
    };
    const providerSessions = [
      {
        dispatch_id: dispatchId,
        outcome: "ACCEPTED",
        provider_session: {
          id: "sess_active",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: selectedNode.transition_id,
        work_items: [workItem],
        workstation_name: selectedNode.workstation_name,
      },
    ];
    const executionDetails = selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID:
        snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions,
      selectedNode,
      workItem,
      workstationRequestsByDispatchID:
        snapshot.runtime.workstation_requests_by_dispatch_id,
    });

    vi.mocked(globalThis.fetch).mockResolvedValue(
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
            id: "sess_active",
            kind: "session_id",
            provider: "codex",
          },
          source: {
            relativePath: "2026/05/18/rollout-sess_active.jsonl",
            sizeBytes: 512,
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

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedWorkDispatchAttempts: providerSessions,
          selectedWorkProviderSessions: providerSessions,
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
      name: "Select provider session codex / session_id / sess_active for dispatch dispatch-review-active",
    });

    expect(selectSessionButton.getAttribute("aria-pressed")).toBe("false");

    await user.click(selectSessionButton);

    expect(within(currentSelection).getByText(workItem.work_id)).toBeTruthy();
    expect(
      within(currentSelection)
        .getByRole("button", {
          name: "Select provider session codex / session_id / sess_active for dispatch dispatch-review-active",
        })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Request details",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Selected session details",
      }),
    ).toBeTruthy();
  });

  it("refreshes the session-detail panel when a different provider-session card is selected", async () => {
    const user = userEvent.setup();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const dispatchId = "dispatch-review-active";
    const execution =
      snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId];
    const workItem = execution?.work_items?.[0];

    if (!execution || !workItem || !selectedNode) {
      throw new Error(
        "expected current-selection fixture for provider-session selection",
      );
    }

    const selection: DashboardSelection = {
      dispatchId,
      execution,
      kind: "work-item",
      nodeId: selectedNode.node_id,
      workItem,
    };
    const providerSessions = [
      {
        dispatch_id: dispatchId,
        outcome: "ACCEPTED",
        provider_session: {
          id: "sess_first",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: selectedNode.transition_id,
        work_items: [workItem],
        workstation_name: selectedNode.workstation_name,
      },
      {
        dispatch_id: `${dispatchId}-retry`,
        outcome: "ACCEPTED",
        provider_session: {
          id: "sess_second",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: selectedNode.transition_id,
        work_items: [workItem],
        workstation_name: selectedNode.workstation_name,
      },
    ];
    const executionDetails = selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID:
        snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions,
      selectedNode,
      workItem,
      workstationRequestsByDispatchID:
        snapshot.runtime.workstation_requests_by_dispatch_id,
    });

    let resolveSecondResponse: ((value: Response) => void) | null = null;
    vi.mocked(globalThis.fetch).mockImplementation((input) => {
      const requestURL = String(input);
      if (requestURL.includes("id=sess_first")) {
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
                id: "sess_first",
                kind: "session_id",
                provider: "codex",
              },
              source: {
                relativePath: "2026/05/18/rollout-sess_first.jsonl",
                sizeBytes: 256,
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
      }

      if (requestURL.includes("id=sess_second")) {
        return new Promise<Response>((resolve) => {
          resolveSecondResponse = resolve;
        });
      }

      throw new Error(`unexpected provider-session request: ${requestURL}`);
    });

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedWorkDispatchAttempts: providerSessions,
          selectedWorkProviderSessions: providerSessions,
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const firstButton = within(currentSelection).getByRole("button", {
      name: "Select provider session codex / session_id / sess_first for dispatch dispatch-review-active",
    });
    const secondButton = within(currentSelection).getByRole("button", {
      name: "Select provider session codex / session_id / sess_second for dispatch dispatch-review-active-retry",
    });

    await user.click(firstButton);

    expect(
      await within(currentSelection).findByText(
        "2026/05/18/rollout-sess_first.jsonl",
      ),
    ).toBeTruthy();

    await user.click(secondButton);

    expect(within(currentSelection).getByText("Loading session details...")).toBeTruthy();
    expect(
      within(currentSelection).queryByText("2026/05/18/rollout-sess_first.jsonl"),
    ).toBeNull();

    resolveSecondResponse?.(
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
            id: "sess_second",
            kind: "session_id",
            provider: "codex",
          },
          source: {
            relativePath: "2026/05/18/rollout-sess_second.jsonl",
            sizeBytes: 384,
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

    expect(
      await within(currentSelection).findByText(
        "2026/05/18/rollout-sess_second.jsonl",
      ),
    ).toBeTruthy();
  });

  it("renders selected state details when a state node is active", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedStatePlace =
      snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
        (place) => place.place_id === "story:complete",
      ) ?? null;

    if (!selectedStatePlace) {
      throw new Error(
        "expected semantic workflow fixture to include a terminal state place",
      );
    }

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedStatePlace,
          selectedStateTerminalHistoryWorkItems: [
            {
              display_name: "Done Story",
              trace_id: "trace-done-story",
              work_id: "work-done-story",
              work_type_id: "story",
            },
          ],
          selectedStateTokenCount: 1,
          selection: {
            kind: "state-node",
            placeId: selectedStatePlace.place_id,
          },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(within(currentSelection).getByTitle("story:complete")).toBeTruthy();
    expect(within(currentSelection).getByText("Current work")).toBeTruthy();
    expect(within(currentSelection).getByText("Done Story")).toBeTruthy();
  });

  it("forwards state-node work-item clicks into the current selection handler", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedStatePlace =
      snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
        (place) => place.place_id === "story:complete",
      ) ?? null;
    const selectStateWorkItem = vi.fn();

    if (!selectedStatePlace) {
      throw new Error(
        "expected semantic workflow fixture to include a terminal state place",
      );
    }

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectStateWorkItem,
          selectedStatePlace,
          selectedStateTerminalHistoryWorkItems: [
            {
              display_name: "Done Story",
              trace_id: "trace-done-story",
              work_id: "work-done-story",
              work_type_id: "story",
            },
          ],
          selectedStateTokenCount: 1,
          selection: {
            kind: "state-node",
            placeId: selectedStatePlace.place_id,
          },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Select work item Done Story" }),
    );

    expect(selectStateWorkItem).toHaveBeenCalledWith(selectedStatePlace, {
      display_name: "Done Story",
      trace_id: "trace-done-story",
      work_id: "work-done-story",
      work_type_id: "story",
    });
  });

  it("renders workstation details when a workstation is active", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions ?? [],
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(vi.mocked(useCurrentEditableFactoryDefinition)).toHaveBeenCalledWith(
      true,
    );
    expect(
      within(currentSelection).getByRole("heading", { name: "Active work" }),
    ).toBeTruthy();
    expect(
      within(currentSelection).getByRole("heading", { name: "Run history" }),
    ).toBeTruthy();
  });

  it("does not load the editable factory definition when no workstation is selected", () => {
    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(vi.mocked(useCurrentEditableFactoryDefinition)).toHaveBeenCalledWith(
      false,
    );
  });

  it("enables editable workstation loading after a workstation becomes selected", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(
      vi.mocked(useCurrentEditableFactoryDefinition),
    ).toHaveBeenLastCalledWith(true);
  });

  it("loads editable workstation inputs when a workstation is already selected on mount", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(vi.mocked(useCurrentEditableFactoryDefinition)).toHaveBeenCalledWith(
      true,
    );

    expandEditableConfiguration();

    expect((screen.getByLabelText("Worker") as HTMLSelectElement).value).toBe(
      "reviewer",
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the latest story changes before approval.",
    );
  });

  it("initializes editable workstation inputs from the canonical factory definition and allows worker edits", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );

    const { rerender } = renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();

    expect((screen.getByLabelText("Worker") as HTMLSelectElement).value).toBe(
      "reviewer",
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the latest story changes before approval.",
    );

    fireEvent.change(screen.getByLabelText("Worker"), {
      target: { value: "planner" },
    });

    expect((screen.getByLabelText("Worker") as HTMLSelectElement).value).toBe(
      "planner",
    );
  });

  it("preserves unsaved editable workstation input when the server definition refreshes", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );

    const { rerender } = renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandEditableConfiguration();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep my local edit." },
    });

    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          prompt: "Server changed prompt",
          workerName: "planner",
        }),
      ),
    );

    rerender(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep my local edit.",
    );
    expect((screen.getByLabelText("Worker") as HTMLSelectElement).value).toBe(
      "reviewer",
    );
    expect(
      screen.getByText(
        "The running factory changed after you started editing. Saving now will overwrite newer server values for prompt, worker.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Review the latest runtime values before saving, or keep editing if this draft should replace them.",
      ),
    ).toBeTruthy();
  });

  it("renders workstation request details when a workstation request is selected", () => {
    const selectedWorkstationRequest = buildDashboardWorkstationRequestFixture(
      "dispatch-review-markdown",
      {
        inference_attempts: [
          buildDashboardInferenceAttemptFixture("dispatch-review-markdown", {
            attempt: 1,
            inference_request_id:
              "dispatch-review-markdown/inference-request/1",
            prompt: [
              "## Review checklist",
              "",
              "- Check the latest diff",
              "- Run `bun test` before approval",
              "",
              "```text",
              "bun test",
              "```",
            ].join("\n"),
          }),
        ],
        request_id: "request-markdown-story",
      },
    );

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode:
            semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id
              .review,
          selectedWorkstationRequest,
          selection: {
            dispatchId: selectedWorkstationRequest.dispatch_id,
            kind: "workstation-request",
            nodeId: selectedWorkstationRequest.workstation_node_id,
            request: selectedWorkstationRequest,
          },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).getAllByText("request-markdown-story").length,
    ).toBeGreaterThan(0);
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeTruthy();
    const inferenceAttempts = within(
      within(currentSelection).getByRole("region", { name: "Inference attempts" }),
    );
    const requestBody = within(
      inferenceAttempts.getByRole("region", { name: "Request body" }),
    );

    expect(
      requestBody.getByRole("heading", {
        level: 2,
        name: "Review checklist",
      }),
    ).toBeTruthy();
    expect(requestBody.getByRole("list")).toBeTruthy();
    expect(requestBody.getByText("Check the latest diff")).toBeTruthy();
    expect(
      requestBody.queryByText("## Review checklist"),
    ).toBeNull();
    expect(requestBody.queryByText("```text")).toBeNull();
    expect(requestBody.getAllByText(/bun test/)).toHaveLength(2);
    expect(
      within(currentSelection).queryByRole("heading", { name: "Active work" }),
    ).toBeNull();
  });

  it("renders the empty current-selection guidance when nothing is selected", () => {
    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(
      screen.getByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeTruthy();
  });

  it("renders localized current-selection shell copy for a supported non-default locale", () => {
    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        locale="ja"
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(
      screen.getByRole("article", {
        name: "現在の選択",
      }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "選択を元に戻す" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "選択をやり直す" })).toBeTruthy();
    expect(
      screen.getByText(
        "ライブの詳細を確認するには、ワークステーション、作業項目、または状態ノードを選択してください。",
      ),
    ).toBeTruthy();
  });

  it("renders disabled undo and redo controls in the shared current-selection header by default", () => {
    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expect(
      screen
        .getByRole("button", { name: "Undo selection" })
        .getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Redo selection" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });
});

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

function renderWithQueryClient(view: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(view, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    ),
  });
}

function expandEditableConfiguration() {
  const section = screen
    .getAllByRole("heading", { name: "Configuration" })
    .at(-1)
    ?.closest("section");
  if (!section) {
    throw new Error("expected editable configuration section");
  }

  fireEvent.click(
    within(section).getByRole("button", {
      name: "Expand editable configuration",
    }),
  );
}

function buildEditableFactoryDefinition(overrides?: {
  prompt?: string;
  workerName?: string;
  workerOptions?: string[];
}): CanonicalFactoryDefinition {
  return {
    name: "Current Factory",
    workers: (overrides?.workerOptions ?? ["reviewer", "planner"]).map(
      (name, index) => ({
        model: `gpt-5.${index + 5}`,
        name,
        type: "MODEL_WORKER",
      }),
    ),
    workstations: [
      {
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: overrides?.workerName ?? "reviewer",
      },
    ],
    workTypes: [],
  };
}
