import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach } from "vitest";

import type { CurrentFactoryDocument } from "../../../../../api/current-factory-definition";
import { dashboardWorkstationRequestFixtures } from "../../../../../components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { CurrentSelectionState } from "../../../hooks/core/useCurrentSelection";
import { selectWorkItemExecutionDetails } from "../../../state/executionDetails";
import type { DashboardSelection } from "../../../state/selection-types";
import { resetSelectionHistoryStore } from "../../../state/selectionHistoryStore";
import { useSaveEditableWorkstationConfiguration } from "../../../workstation-selection/hooks/use-save-editable-workstation-configuration";
import { useCurrentWorkstationPromptTemplateValidation } from "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation";
import { CurrentSelectionWidget } from "../../widget/current-selection-widget";
import {
  createCurrentSelectionWidgetQueryClient,
  renderWithExistingQueryClient,
} from "../../widget/current-selection-widget-test-utils";

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
const activeWorkLabel = "Active Story";

function getCurrentSelectionPrimaryTitle(
  currentSelection: HTMLElement,
): HTMLElement {
  const primaryTitle = currentSelection.querySelector(".type-display-large");

  if (!(primaryTitle instanceof HTMLElement)) {
    throw new Error("expected current selection primary title");
  }

  return primaryTitle;
}

function expectCanonicalCurrentSelectionBody({
  currentSelection,
  sectionNames,
  title,
}: {
  currentSelection: HTMLElement;
  sectionNames: string[];
  title: string;
}): void {
  const primaryTitle = getCurrentSelectionPrimaryTitle(currentSelection);
  expect(primaryTitle.textContent).toBe(title);

  const body = primaryTitle.parentElement;
  if (!(body instanceof HTMLElement)) {
    throw new Error("expected current selection body layout");
  }

  const topLevelSections = Array.from(body.children).filter(
    (child): child is HTMLElement =>
      child instanceof HTMLElement && child.tagName.toLowerCase() === "section",
  );
  expect(topLevelSections.length).toBeGreaterThan(0);

  for (const sectionName of sectionNames) {
    const section = topLevelSections.find((candidate) =>
      within(candidate).queryByRole("heading", { name: sectionName }),
    );
    if (!(section instanceof HTMLElement)) {
      throw new Error(
        `expected top-level current selection section ${sectionName}`,
      );
    }
    expect(topLevelSections).toContain(section);

    const toggle = within(section)
      .getAllByRole("button")
      .find((button) => button.getAttribute("aria-expanded") !== null);
    if (!(toggle instanceof HTMLElement)) {
      throw new Error(`expected disclosure button for ${sectionName}`);
    }
    expect(toggle.tagName.toLowerCase()).toBe("button");
    expect(toggle.getAttribute("aria-expanded")).not.toBeNull();

    const previousExpanded = toggle.getAttribute("aria-expanded");
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).not.toBe(previousExpanded);
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe(previousExpanded);
  }
}

function buildCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  const currentFactoryDefinition =
    overrides.currentFactoryDefinition ?? buildEditableFactoryDefinition();
  const selectedWorkerName =
    overrides.selectedWorkerName ??
    (overrides.selection?.kind === "worker"
      ? overrides.selection.workerName
      : null);
  const selectedResourceName =
    overrides.selectedResourceName ??
    (overrides.selection?.kind === "resource"
      ? overrides.selection.resourceName
      : null);

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
    selectedWorkOperationHistory: [],
    selectedWorkProviderSessions: [],
    selectedWorkRequestHistory: [],
    selectedWorkWorkstationRequests: [],
    selectedResource:
      currentFactoryDefinition.resources?.find(
        (resource) => resource.name === selectedResourceName,
      ) ?? null,
    selectedResourceName,
    selectedResourceTokenCount: null,
    selectedResourceWorkerNames: [],
    selectedResourceWorkstationNames: [],
    selectedWorker:
      currentFactoryDefinition.workers?.find(
        (worker) => worker.name === selectedWorkerName,
      ) ?? null,
    selectedWorkerName,
    selectedWorkerWorkstationNames: [],
    selectedWorkType: null,
    selectedWorkTypeName: null,
    selectedWorkstationRequest: null,
    selection: null,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkItem: () => undefined,
    selectResource: () => undefined,
    selectWorker: () => undefined,
    selectWorkType: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    selectWorkByID: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

function buildEditableFactoryDefinition(): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "4",
      physical: "2026-05-20T03:45:00Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        name: "planner",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        behavior: "STANDARD",
        body: "Review the active story before approval.",
        id: "review",
        inputs: [{ state: "implemented", workType: "story" }],
        name: "Review",
        outputs: [{ state: "complete", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: "reviewer",
      },
    ],
    workTypes: [],
  };
}

function buildEditableDefinitionResult(data: CurrentFactoryDocument) {
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

type CanonicalLayoutCase = {
  currentSelection: CurrentSelectionState;
  sectionNames: string[];
  title: string;
};

function buildCanonicalLayoutCases(): CanonicalLayoutCase[] {
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
  const selectedStatePlace =
    selectedNode.input_places?.find(
      (place) => place.place_id === "story:implemented",
    ) ?? null;
  const readyRequest = dashboardWorkstationRequestFixtures.ready;

  if (!execution || !workItem || !selectedNode || !selectedStatePlace) {
    throw new Error("expected semantic workflow current-selection fixtures");
  }

  const workItemSelection: DashboardSelection = {
    dispatchId,
    execution,
    kind: "work-item",
    nodeId: selectedNode.node_id,
    workItem,
  };
  const requestSelection: DashboardSelection = {
    dispatchId: readyRequest.dispatch_id,
    kind: "workstation-request",
    nodeId: readyRequest.workstation_node_id,
    request: readyRequest,
  };

  return [
    {
      currentSelection: buildCurrentSelection({
        selectedNode,
        selectedNodeActiveExecutions: [execution],
        selectedNodeProviderSessions: providerSessions ?? [],
        selection: { kind: "node", nodeId: selectedNode.node_id },
      }),
      sectionNames: [
        "Workstation summary",
        "Configuration",
        "Active work",
        "Run history",
      ],
      title: "Review",
    },
    {
      currentSelection: buildCurrentSelection({
        selectedNode,
        selectedWorkDispatchAttempts: providerSessions ?? [],
        selectedWorkProviderSessions: providerSessions ?? [],
        selectedWorkRequestHistory,
        selection: workItemSelection,
      }),
      sectionNames: [
        "Summary",
        "Work contents",
        "Work relationships",
        "Work operations",
      ],
      title: activeWorkLabel,
    },
    {
      currentSelection: buildCurrentSelection({
        selectedNode,
        selectedWorkstationRequest: readyRequest,
        selection: requestSelection,
      }),
      sectionNames: [
        "Summary",
        "Request details",
        "Response details",
        "Inference attempts",
      ],
      title: activeWorkLabel,
    },
    {
      currentSelection: buildCurrentSelection({
        currentFactoryDefinition: {
          ...buildEditableFactoryDefinition(),
          workTypes: [
            {
              name: "story",
              states: [
                { name: "implemented", type: "PROCESSING" },
                { name: "complete", type: "TERMINAL" },
              ],
            },
          ],
        },
        selectedNode,
        selectedStateCurrentWorkItems: [
          {
            display_name: activeWorkLabel,
            started_at: "2026-04-08T12:00:01Z",
            trace_id: "trace-active-story",
            work_id: "work-active-story",
            work_type_id: "story",
          },
        ],
        selectedStatePlace,
        selectedStateTokenCount: 1,
        selection: {
          kind: "state",
          nodeId: selectedNode.node_id,
          placeId: selectedStatePlace.place_id,
        },
      }),
      sectionNames: ["Summary", "Current work"],
      title: "story: implemented",
    },
  ];
}

describe("CurrentSelectionWidget canonical section layout", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
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
    vi.mocked(useSaveEditableWorkstationConfiguration).mockReturnValue({
      canSave: false,
      save: vi.fn(),
      saveState: { status: "idle" },
    });
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
  });

  afterEach(() => {
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
    resetSelectionHistoryStore();
  });

  it("keeps every current-selection kind on the canonical title and expandable section layout", () => {
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

    if (!execution || !workItem || !selectedNode) {
      throw new Error("expected semantic workflow current-selection fixtures");
    }

    for (const {
      currentSelection,
      sectionNames,
      title,
    } of buildCanonicalLayoutCases()) {
      cleanup();
      const executionDetails =
        currentSelection.selection?.kind === "work-item"
          ? selectWorkItemExecutionDetails({
              activeExecution: execution,
              dispatchID: dispatchId,
              inferenceAttemptsByDispatchID:
                snapshot.runtime.inference_attempts_by_dispatch_id,
              providerSessions: providerSessions ?? [],
              selectedNode,
              workItem,
              workstationRequestsByDispatchID:
                snapshot.runtime.workstation_requests_by_dispatch_id,
            })
          : null;

      renderWithExistingQueryClient(
        queryClient,
        <CurrentSelectionWidget
          currentSelection={currentSelection}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={executionDetails}
        />,
      );

      expectCanonicalCurrentSelectionBody({
        currentSelection: screen.getByRole("article", {
          name: "Current selection",
        }),
        sectionNames,
        title,
      });
    }
  });
});
