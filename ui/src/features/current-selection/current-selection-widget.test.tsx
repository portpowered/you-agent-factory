import { fireEvent, render, screen, within } from "@testing-library/react";
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
import type { CurrentSelectionState } from "./useCurrentSelection";

vi.mock("../current-factory-definition", async () => {
  const actual = await vi.importActual("../current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinition: vi.fn(),
  };
});

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
  });

  afterEach(() => {
    resetSelectionHistoryStore();
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

    render(
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

    render(
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

    render(
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

    render(
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

    render(
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
      false,
    );
    expect(
      within(currentSelection).getByRole("heading", { name: "Active work" }),
    ).toBeTruthy();
    expect(
      within(currentSelection).getByRole("heading", { name: "Run history" }),
    ).toBeTruthy();
  });

  it("does not load the editable factory definition when no workstation is selected", () => {
    render(
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
    const { rerender } = render(
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

  it("initializes editable workstation inputs from the canonical factory definition and validates local edits", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );

    const { rerender } = render(
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

    expect((screen.getByLabelText("Model") as HTMLInputElement).value).toBe(
      "gpt-5.5",
    );
    expect((screen.getByLabelText("Template") as HTMLInputElement).value).toBe(
      "prompts/review.md",
    );
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the latest story changes before approval.",
    );

    fireEvent.change(screen.getByLabelText("Model"), {
      target: { value: "   " },
    });

    expect(
      screen.getByText("Enter a model before saving this workstation."),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Resolve the highlighted fields before saving this workstation.",
      ),
    ).toBeTruthy();
  });

  it("preserves unsaved editable workstation input when the server definition refreshes", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );

    const { rerender } = render(
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

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep my local edit." },
    });

    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          model: "gpt-5.6",
          prompt: "Server changed prompt",
          promptFile: "prompts/server.md",
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
    expect((screen.getByLabelText("Model") as HTMLInputElement).value).toBe(
      "gpt-5.5",
    );
    expect((screen.getByLabelText("Template") as HTMLInputElement).value).toBe(
      "prompts/review.md",
    );
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

    render(
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
    expect(
      within(currentSelection).getByText(/## Review checklist/),
    ).toBeTruthy();
    expect(
      within(currentSelection).getByText(/- Check the latest diff/),
    ).toBeTruthy();
    expect(within(currentSelection).getByText(/```text/)).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", { name: "Active work" }),
    ).toBeNull();
  });

  it("renders the empty current-selection guidance when nothing is selected", () => {
    render(
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
    render(
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
    render(
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

function buildEditableFactoryDefinition(overrides?: {
  model?: string;
  prompt?: string;
  promptFile?: string;
}): CanonicalFactoryDefinition {
  return {
    name: "Current Factory",
    workers: [
      {
        model: overrides?.model ?? "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: overrides?.promptFile ?? "prompts/review.md",
        worker: "reviewer",
      },
    ],
    workTypes: [],
  };
}
