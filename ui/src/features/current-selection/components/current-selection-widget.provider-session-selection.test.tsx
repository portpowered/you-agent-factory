import "../../../../testing/bun-current-selection-widget-chrome-mocks";
import { beforeEach, describe, expect, it, vi } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  buildDashboardInferenceAttemptFixture,
  buildDashboardWorkstationRequestFixture,
} from "../../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDefinitionMock } from "../../../../testing/bun-current-factory-definition-public-mocks";
import {
  useCurrentWorkstationPromptTemplateContractMock,
  useCurrentWorkstationPromptTemplateValidationMock,
  useSaveEditableWorkstationConfigurationMock,
} from "../../../../testing/bun-current-selection-widget-chrome-mocks";
import { providerSessionSelectionKey } from "../../provider-session-detail/lib/provider-session-ref";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";
import { resetSelectionHistoryStore } from "../state/selectionHistoryStore";
import type { DashboardSelection } from "../state/selection-types";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { CurrentSelectionWidget } from "./current-selection-widget";
import {
  createCurrentSelectionWidgetQueryClient,
  renderWithQueryClient,
  wrapCurrentSelectionWidgetView,
} from "./current-selection-widget-test-utils";

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
    useCurrentWorkstationPromptTemplateContractMock.mockReturnValue({
      data: {
        availableVariables: [],
        inputCount: 0,
        unavailableAccessPatterns: [],
      },
      error: null,
      isError: false,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);
    useCurrentWorkstationPromptTemplateValidationMock.mockReturnValue({
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
    useCurrentFactoryDefinitionMock.mockReturnValue({
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
    useSaveEditableWorkstationConfigurationMock.mockReturnValue({
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

  it("keeps an inference-attempt-only provider session selected in current-selection state without rendering duplicate detail", async () => {
    const user = userEvent.setup();
    const {
      dispatchId,
      execution,
      selectedNode,
      selection,
      snapshot,
      workItem,
    } = buildSelectedWorkItemFixture();
    const inferenceOnlyRequest = buildDashboardWorkstationRequestFixture(
      dispatchId,
      {
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
      },
    );
    const executionDetails = selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID:
        snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions: [],
      selectedNode,
      workItem,
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
    const inferenceAttempts = within(currentSelection).getByRole("region", {
      name: "Inference attempts",
    });
    await user.click(
      within(inferenceAttempts).getByRole("button", { name: "Expand" }),
    );
    await user.click(
      within(inferenceAttempts).getByRole("button", {
        name: "Expand attempt 1",
      }),
    );
    const selectSessionButton = within(currentSelection).getByRole("button", {
      name: "Select provider session codex / Session ID / sess_inference_only for dispatch dispatch-review-active",
    });

    expect(selectSessionButton.getAttribute("aria-pressed")).toBe("false");

    await user.click(selectSessionButton);

    expect(
      within(currentSelection)
        .getByRole("button", {
          name: "Select provider session codex / Session ID / sess_inference_only for dispatch dispatch-review-active",
        })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Selected session details",
      }),
    ).toBeNull();
  });

  it("keeps selection controls available for timestamp-prefixed Codex session files without embedding the detail panel", async () => {
    const user = userEvent.setup();
    const {
      dispatchId,
      execution,
      selectedNode,
      selection,
      snapshot,
      workItem,
    } = buildSelectedWorkItemFixture();
    const codexSessionID = "019e44f4-580e-7f32-981e-1e54ec6907d6";
    const requestWithTimestampPrefixedSession =
      buildDashboardWorkstationRequestFixture(dispatchId, {
        inference_attempts: [
          buildDashboardInferenceAttemptFixture(dispatchId, {
            outcome: "SUCCEEDED",
            provider_session: {
              id: codexSessionID,
              kind: "session_id",
              provider: "codex",
            },
            response: "Resolved from the on-disk Codex session artifact.",
          }),
        ],
      });
    const executionDetails = selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID:
        snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions: [],
      selectedNode,
      workItem,
    });

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedWorkDispatchAttempts: [],
          selectedWorkProviderSessions: [],
          selectedWorkRequestHistory: [requestWithTimestampPrefixedSession],
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const inferenceAttempts = within(currentSelection).getByRole("region", {
      name: "Inference attempts",
    });
    await user.click(
      within(inferenceAttempts).getByRole("button", { name: "Expand" }),
    );
    await user.click(
      within(inferenceAttempts).getByRole("button", {
        name: "Expand attempt 1",
      }),
    );

    await user.click(
      within(currentSelection).getByRole("button", {
        name: `Select provider session codex / Session ID / ${codexSessionID} for dispatch dispatch-review-active`,
      }),
    );

    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Selected session details",
      }),
    ).toBeNull();
  });

  it("updates which session is selected when switching between inference-attempt sessions without showing duplicate detail", async () => {
    const user = userEvent.setup();
    const { dispatchId, execution, selectedNode, selection, workItem } =
      buildSelectedWorkItemFixture();
    const requestWithSelectableInferenceAttempts =
      buildDashboardWorkstationRequestFixture(dispatchId, {
        inference_attempts: [
          buildDashboardInferenceAttemptFixture(dispatchId, {
            outcome: "SUCCEEDED",
            provider_session: {
              id: "sess_inference_first",
              kind: "session_id",
              provider: "codex",
            },
            response: "First inference-attempt session.",
          }),
          buildDashboardInferenceAttemptFixture(dispatchId, {
            attempt: 2,
            outcome: "SUCCEEDED",
            provider_session: {
              id: "sess_inference_second",
              kind: "session_id",
              provider: "codex",
            },
            response: "Second inference-attempt session.",
          }),
        ],
      });
    const executionDetails = selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID: {},
      providerSessions: [],
      selectedNode,
      workItem,
    });

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedWorkDispatchAttempts: [],
          selectedWorkProviderSessions: [],
          selectedWorkRequestHistory: [requestWithSelectableInferenceAttempts],
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const inferenceAttempts = within(currentSelection).getByRole("region", {
      name: "Inference attempts",
    });
    await user.click(
      within(inferenceAttempts).getByRole("button", { name: "Expand" }),
    );
    await user.click(
      within(inferenceAttempts).getByRole("button", {
        name: "Expand attempt 1",
      }),
    );
    await user.click(
      within(inferenceAttempts).getByRole("button", {
        name: "Expand attempt 2",
      }),
    );
    const firstButton = within(currentSelection).getByRole("button", {
      name: "Select provider session codex / Session ID / sess_inference_first for dispatch dispatch-review-active",
    });
    const secondButton = within(currentSelection).getByRole("button", {
      name: "Select provider session codex / Session ID / sess_inference_second for dispatch dispatch-review-active",
    });

    await user.click(firstButton);

    expect(firstButton.getAttribute("aria-pressed")).toBe("true");
    expect(secondButton.getAttribute("aria-pressed")).toBe("false");
    expect(
      within(currentSelection).queryByText(
        "2026/05/18/rollout-sess_inference_first.jsonl",
      ),
    ).toBeNull();

    await user.click(secondButton);

    expect(firstButton.getAttribute("aria-pressed")).toBe("false");
    expect(secondButton.getAttribute("aria-pressed")).toBe("true");
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Selected session details",
      }),
    ).toBeNull();
  });

  it("routes workstation run-history provider-session selection through shared selection state", async () => {
    const user = userEvent.setup();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );
    const resolvedProviderSessions = providerSessions ?? [];
    const onSelectProviderSession = vi.fn();
    const selectedSession = {
      dispatchID: "dispatch-review-active",
      id: "sess-active-story",
      kind: "session_id",
      provider: "codex",
    } as const;
    const currentSelection = buildCurrentSelection({
      selectedNode,
      selectedNodeProviderSessions: resolvedProviderSessions,
      selection: { kind: "node", nodeId: selectedNode.node_id },
    });

    const queryClient = createCurrentSelectionWidgetQueryClient();
    const renderCurrentSelection = (
      selectedProviderSessionKey: string | null,
    ) =>
      wrapCurrentSelectionWidgetView(
        queryClient,
        <CurrentSelectionWidget
          currentSelection={currentSelection}
          now={DETAIL_CARD_NOW}
          onSelectProviderSession={onSelectProviderSession}
          selectedProviderSessionKey={selectedProviderSessionKey}
          selectedWorkExecutionDetails={null}
        />,
      );
    const { rerender } = render(renderCurrentSelection(null));

    const currentSelectionCard = screen.getByRole("article", {
      name: "Current selection",
    });
    const runHistory = within(currentSelectionCard).getByRole("region", {
      name: "Run history",
    });
    await user.click(
      within(runHistory).getByRole("button", { name: "Expand" }),
    );
    const selectSessionButton = within(runHistory).getByRole("button", {
      name: "Select provider session codex / Session ID / sess-active-story for dispatch dispatch-review-active",
    });

    expect(selectSessionButton.getAttribute("aria-pressed")).toBe("false");

    await user.click(selectSessionButton);

    expect(onSelectProviderSession).toHaveBeenCalledWith(selectedSession);

    rerender(
      renderCurrentSelection(providerSessionSelectionKey(selectedSession)),
    );

    expect(
      within(runHistory)
        .getByRole("button", {
          name: "Select provider session codex / Session ID / sess-active-story for dispatch dispatch-review-active",
        })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      within(currentSelectionCard).queryByRole("heading", {
        name: "Selected session details",
      }),
    ).toBeNull();
  });
});

