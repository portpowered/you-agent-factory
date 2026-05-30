import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  getCurrentFactoryWorkstationPromptTemplateContract,
  validateCurrentFactoryWorkstationPromptTemplate,
  type PromptTemplateContract,
  type PromptTemplateValidationResult,
} from "../../../api/current-factory-prompt-template";
import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} from "../../current-factory-definition/public";
import { resetSelectionHistoryStore } from "../state/selectionHistoryStore";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { CurrentSelectionWidget } from "./current-selection-widget";

const saveCurrentFactoryMutation = vi.fn();

const promptTemplateContract: PromptTemplateContract = {
  availableVariables: [
    {
      category: "ROOT",
      description: "The current work item identifier.",
      example: "{{ .WorkID }}",
      path: ".WorkID",
    },
  ],
  inputCount: 1,
  unavailableAccessPatterns: [],
};

const validPromptValidation: PromptTemplateValidationResult = {
  diagnostics: [],
  valid: true,
};

vi.mock("../../../api/current-factory-prompt-template", async () => {
  const actual = await vi.importActual<
    typeof import("../../../api/current-factory-prompt-template")
  >("../../../api/current-factory-prompt-template");

  return {
    ...actual,
    getCurrentFactoryWorkstationPromptTemplateContract: vi.fn(),
    validateCurrentFactoryWorkstationPromptTemplate: vi.fn(),
  };
});

vi.mock("../../current-factory-definition/public", async () => {
  const actual = await vi.importActual("../../current-factory-definition/public");

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
    useSaveCurrentFactory: vi.fn(),
  };
});

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

describe("CurrentSelectionWidget prompt-edit save enablement", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
    vi.mocked(useSaveCurrentFactory).mockReturnValue({
      isPending: false,
      mutateAsync: saveCurrentFactoryMutation,
    } as never);
    vi.mocked(getCurrentFactoryWorkstationPromptTemplateContract).mockImplementation(
      async () => promptTemplateContract,
    );
    vi.mocked(validateCurrentFactoryWorkstationPromptTemplate).mockImplementation(
      async (_workstationName, prompt) => ({
        ...validPromptValidation,
        diagnostics: [],
        valid: prompt.trim().length > 0,
      }),
    );
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("enables save and opens overwrite confirmation after a valid prompt-only edit", async () => {
    renderWorkstationSelection();

    expandEditableConfiguration();

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton.getAttribute("disabled")).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated review instructions for save." },
    });

    await waitFor(() => {
      expect(saveButton.getAttribute("disabled")).toBeNull();
    });

    fireEvent.click(saveButton);

    expect(
      await screen.findByRole("heading", {
        name: "Overwrite the running factory definition?",
      }),
    ).toBeTruthy();
    expect(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Overwrite factory",
      }),
    ).toBeTruthy();
  });
});

function expandEditableConfiguration(
  buttonName = "Expand editable configuration",
  headingName = "Configuration",
) {
  const section = screen
    .getAllByRole("heading", { name: headingName })
    .at(-1)
    ?.closest("section");
  if (!section) {
    throw new Error("expected editable configuration section");
  }

  fireEvent.click(
    within(section).getByRole("button", {
      name: buttonName,
    }),
  );
}

function renderWorkstationSelection() {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

  return renderWithQueryClient(
    <CurrentSelectionWidget
      currentSelection={buildCurrentSelection({
        selectedNode,
        selection: { kind: "node", nodeId: selectedNode.node_id },
      })}
      now={DETAIL_CARD_NOW}
      selectedWorkExecutionDetails={null}
    />,
  );
}

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
  data: CurrentFactoryDocument | undefined,
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

function buildEditableFactoryDefinition(): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
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
}

function renderWithQueryClient(view: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}
