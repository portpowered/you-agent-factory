import "../../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { CurrentSelectionState } from "../../../hooks/core/useCurrentSelection";
import { resetSelectionHistoryStore } from "../../../state/selectionHistoryStore";
import { useSaveEditableWorkTypeConfiguration } from "../../../work-type-selection/hooks/use-save-editable-work-type-configuration";
import { useSaveEditableWorkstationConfiguration } from "../../../workstation-selection/hooks/use-save-editable-workstation-configuration";
import { useCurrentWorkstationPromptTemplateValidation } from "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation";
import { CurrentSelectionWidget } from "../../widget/current-selection-widget";
import { renderWithQueryClient } from "../../widget/current-selection-widget-test-utils";

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
  "../../../work-type-selection/hooks/use-save-editable-work-type-configuration",
  () => ({
    useSaveEditableWorkTypeConfiguration: vi.fn(),
  }),
);

vi.mock(
  "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation",
  () => ({
    useCurrentWorkstationPromptTemplateValidation: vi.fn(),
  }),
);

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

function buildCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  return {
    canRedoSelection: false,
    canUndoSelection: false,
    completedWorkItems: [],
    currentFactoryDefinition: null,
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
    selectedWorker: null,
    selectedWorkerName: null,
    selectedWorkerWorkstationNames: [],
    selectedWorkType: null,
    selectedWorkTypeName: null,
    selection: null,
    selectWorkByID: () => undefined,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkItem: () => undefined,
    selectWorker: () => undefined,
    selectWorkType: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

describe("CurrentSelectionWidget work-type selection", () => {
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
    vi.mocked(useSaveEditableWorkstationConfiguration).mockReturnValue({
      canSave: false,
      save: vi.fn(),
      saveState: { status: "idle" },
    });
    vi.mocked(useSaveEditableWorkTypeConfiguration).mockReturnValue({
      beginSaveConfirmation: vi.fn(),
      canSave: false,
      cancelSaveConfirmation: vi.fn(),
      confirmSave: vi.fn(),
      saveState: { status: "idle" },
    });
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      isError: false,
      isPending: true,
      status: "pending",
    } as never);
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("renders the work type detail card without empty-state guidance", async () => {
    const currentFactoryDefinition = buildEditableFactoryDefinition({
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
    });
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(currentFactoryDefinition),
    );

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          currentFactoryDefinition,
          selectedWorkTypeName: "story",
          selection: { kind: "work-type", workTypeName: "story" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });

    expect(
      screen.queryByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeNull();
    expect(
      (
        await within(currentSelection).findByLabelText("Work type")
      ).getAttribute("value"),
    ).toBe("story");
    expect(
      within(currentSelection).getByRole("heading", { name: "States" }),
    ).toBeTruthy();
    expect(within(currentSelection).getByText("queued")).toBeTruthy();
    expect(within(currentSelection).getByText("Initial")).toBeTruthy();
  });

  it("forwards work-state row navigation to selectWorkstation with the graph node id", async () => {
    const selectWorkstation = vi.fn();
    const currentFactoryDefinition = buildEditableFactoryDefinition({
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(currentFactoryDefinition),
    );

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          currentFactoryDefinition,
          selectedWorkTypeName: "story",
          selection: { kind: "work-type", workTypeName: "story" },
          selectWorkstation,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Select queued state on factory graph",
      }),
    );

    expect(selectWorkstation).toHaveBeenCalledWith("work-state:story:queued");
  });
});

function buildEditableDefinitionResult(
  definition: CurrentFactoryDocument,
): ReturnType<typeof useCurrentFactoryDocument> {
  return {
    data: definition,
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
    isStale: false,
    isSuccess: true,
    promise: Promise.resolve(definition),
    refetch: vi.fn(),
    status: "success",
  } as never;
}

function buildEditableFactoryDefinition(overrides?: {
  workTypes?: CurrentFactoryDocument["workTypes"];
}): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CODEX",
        name: "reviewer",
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
    workTypes: overrides?.workTypes ?? [],
  };
}
