import "../../../../../testing/vitest-dom-capabilities.setup";

import { screen, within } from "@testing-library/react";

import type { CanonicalFactoryDefinition } from "../../../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { CurrentSelectionState } from "../../../hooks/core/useCurrentSelection";
import { selectWorkItemExecutionDetails } from "../../../state/executionDetails";
import { resetSelectionHistoryStore } from "../../../state/selectionHistoryStore";
import { useSaveEditableWorkstationConfiguration } from "../../../workstation-selection/hooks/use-save-editable-workstation-configuration";
import { useCurrentWorkstationPromptTemplateValidation } from "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation";
import { CurrentSelectionWidget } from "../../widget/current-selection-widget";
import {
  createCurrentSelectionWidgetQueryClient,
  renderWithExistingQueryClient,
} from "../../widget/current-selection-widget-test-utils";

const graphBulkSelectionBridgeState = vi.hoisted(() => ({
  value: null as {
    bulkSelectionSummary: {
      totalCount: number;
      kindCounts: Array<{ count: number; kind: string }>;
    } | null;
    primaryTarget: { kind: "node"; id: string } | null;
    selectedEdgeIds: string[];
    selectedNodeIds: string[];
  } | null,
}));

vi.mock(
  "../../../../workflow-activity/state/factory-graph-editor-selection-bridge",
  () => ({
    useFactoryGraphEditorSelectionBridge: (
      selector: (state: {
        selection: typeof graphBulkSelectionBridgeState.value;
      }) => unknown,
    ) =>
      selector({
        selection: graphBulkSelectionBridgeState.value,
      }),
  }),
);

vi.mock(
  "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../../../workstation-selection/hooks/use-save-editable-workstation-configuration",
  () => ({
    useSaveEditableWorkstationConfiguration: vi.fn(),
  }),
);

vi.mock(
  "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation",
  () => ({
    useCurrentWorkstationPromptTemplateValidation: vi.fn(),
  }),
);

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

describe("CurrentSelectionWidget graph bulk selection summary", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    graphBulkSelectionBridgeState.value = null;
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
      buildEditableDefinitionResult(semanticWorkflowDashboardSnapshot.factory),
    );
    vi.mocked(useSaveEditableWorkstationConfiguration).mockReturnValue({
      canSave: false,
      save: vi.fn(),
      saveState: { status: "idle" },
    });
  });

  afterEach(() => {
    resetSelectionHistoryStore();
    graphBulkSelectionBridgeState.value = null;
  });

  it("shows workstation detail for a single graph selection", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const queryClient = createCurrentSelectionWidgetQueryClient();

    renderWithExistingQueryClient(
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

    const panel = screen.getByRole("article", { name: "Current selection" });
    expect(
      within(panel).getByRole("heading", { name: "Configuration" }),
    ).toBeTruthy();
    expect(
      within(panel).queryByText("Multiple graph items selected"),
    ).toBeNull();
  });
});

describe("CurrentSelectionWidget graph bulk selection restoration", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    graphBulkSelectionBridgeState.value = null;
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
      buildEditableDefinitionResult(semanticWorkflowDashboardSnapshot.factory),
    );
    vi.mocked(useSaveEditableWorkstationConfiguration).mockReturnValue({
      canSave: false,
      save: vi.fn(),
      saveState: { status: "idle" },
    });
  });

  afterEach(() => {
    resetSelectionHistoryStore();
    graphBulkSelectionBridgeState.value = null;
  });

  it("shows bulk-selection summary for multi-selection and restores single detail when reduced", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const queryClient = createCurrentSelectionWidgetQueryClient();

    graphBulkSelectionBridgeState.value = {
      bulkSelectionSummary: {
        totalCount: 3,
        kindCounts: [
          { kind: "workstation", count: 2 },
          { kind: "edge", count: 1 },
        ],
      },
      primaryTarget: { kind: "node", id: "workstation:review" },
      selectedEdgeIds: ["edge-review-done"],
      selectedNodeIds: ["workstation:review", "workstation:done"],
    };

    const { rerender } = renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        key="bulk"
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );
    const panel = screen.getByRole("article", { name: "Current selection" });

    expect(
      within(panel).getByText("Multiple graph items selected"),
    ).toBeTruthy();
    expect(within(panel).getByText("Selected items")).toBeTruthy();
    expect(within(panel).getByText("3")).toBeTruthy();
    expect(
      within(panel).queryByRole("heading", { name: "Configuration" }),
    ).toBeNull();

    graphBulkSelectionBridgeState.value = null;

    rerender(
      <CurrentSelectionWidget
        key="single"
        currentSelection={buildCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    const restoredPanel = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(restoredPanel).queryByText("Multiple graph items selected"),
    ).toBeNull();
    expect(
      within(restoredPanel).getByRole("heading", { name: "Configuration" }),
    ).toBeTruthy();
  });
});

describe("CurrentSelectionWidget graph bulk selection non-graph override", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    graphBulkSelectionBridgeState.value = null;
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
      buildEditableDefinitionResult(semanticWorkflowDashboardSnapshot.factory),
    );
    vi.mocked(useSaveEditableWorkstationConfiguration).mockReturnValue({
      canSave: false,
      save: vi.fn(),
      saveState: { status: "idle" },
    });
  });

  afterEach(() => {
    resetSelectionHistoryStore();
    graphBulkSelectionBridgeState.value = null;
  });

  it("shows work-item detail instead of stale graph bulk summary", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const dispatchId = snapshot.runtime.active_dispatch_ids?.[0] ?? "";
    const execution =
      snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId];
    const workItem = execution?.work_items?.[0];
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const queryClient = createCurrentSelectionWidgetQueryClient();

    if (!execution || !workItem || !selectedNode) {
      throw new Error(
        "expected semantic workflow fixture to include an active selected work item",
      );
    }

    graphBulkSelectionBridgeState.value = {
      bulkSelectionSummary: {
        totalCount: 2,
        kindCounts: [{ kind: "workstation", count: 2 }],
      },
      primaryTarget: { kind: "node", id: "workstation:review" },
      selectedEdgeIds: [],
      selectedNodeIds: ["workstation:review", "workstation:done"],
    };

    renderWithExistingQueryClient(
      queryClient,
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedWorkID: workItem.work_id,
          selection: {
            dispatchId,
            execution,
            kind: "work-item",
            nodeId: selectedNode.node_id,
            workItem,
          },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID: dispatchId,
          inferenceAttemptsByDispatchID:
            snapshot.runtime.inference_attempts_by_dispatch_id,
          providerSessions: [],
          selectedNode,
          workItem,
          workstationRequestsByDispatchID:
            snapshot.runtime.workstation_requests_by_dispatch_id,
        })}
      />,
    );

    const panel = screen.getByRole("article", { name: "Current selection" });
    expect(
      within(panel).queryByText("Multiple graph items selected"),
    ).toBeNull();
    expect(within(panel).getByText(workItem.work_id)).toBeTruthy();
  });
});

function buildCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  const currentFactoryDefinition =
    overrides.currentFactoryDefinition ??
    (semanticWorkflowDashboardSnapshot.factory as CanonicalFactoryDefinition);

  return {
    canRedoSelection: false,
    canUndoSelection: false,
    completedWorkItems: [],
    currentFactoryDefinition,
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
    selectedResource: null,
    selectedResourceName: null,
    selectedResourceTokenCount: null,
    selectedResourceWorkerNames: [],
    selectedResourceWorkstationNames: [],
    selectedWorker: null,
    selectedWorkerName: null,
    selectedWorkerWorkstationNames: [],
    selectedWorkType: null,
    selectedWorkTypeName: null,
    selectedWorkstationRequest: null,
    selection: null,
    selectResource: () => undefined,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkByID: () => undefined,
    selectWorkItem: () => undefined,
    selectWorker: () => undefined,
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
