import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type { FactoryValue } from "../../api/named-factory";
import { createFactory, NamedFactoryAPIError } from "../../api/named-factory";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import type { CanonicalFactoryDefinition } from "../current-factory-definition";
import { useCurrentEditableFactoryDefinition } from "../current-factory-definition";
import { CurrentSelectionWidget } from "./current-selection-widget";
import { resetSelectionHistoryStore } from "./state/selectionHistoryStore";
import type { CurrentSelectionState } from "./useCurrentSelection";

vi.mock("../current-factory-definition", async () => {
  const actual = await vi.importActual("../current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinition: vi.fn(),
  };
});

vi.mock("../../api/named-factory", async () => {
  const actual = await vi.importActual("../../api/named-factory");

  return {
    ...actual,
    createFactory: vi.fn(),
  };
});

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

describe("CurrentSelectionWidget workstation save flow", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("keeps the header save action disabled until the workstation draft changes", () => {
    vi.mocked(createFactory).mockResolvedValue(
      buildEditableFactoryDefinition(),
    );

    renderWorkstationSelection();

    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated review instructions." },
    });

    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("confirms before saving and refreshes the form to the saved workstation values", async () => {
    const savedFactory = buildEditableFactoryDefinition({
      prompt: "Review the diff and verify browser behavior.",
    });
    vi.mocked(createFactory).mockResolvedValue(savedFactory);

    renderWorkstationSelection();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Review the diff and verify browser behavior." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      screen.getByRole("heading", {
        name: "Overwrite the running factory definition?",
      }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(createFactory).toHaveBeenCalledWith(
        expect.objectContaining({
          workstations: [
            expect.objectContaining({
              body: "Review the diff and verify browser behavior.",
            }),
          ],
        }),
      );
    });
    await waitFor(() => {
      expect(
        screen.getByText(
          "Running factory saved. The editable workstation values were refreshed to the saved definition.",
        ),
      ).toBeTruthy();
    });

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Review the diff and verify browser behavior.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("preserves edited workstation input when the save request fails", async () => {
    vi.mocked(createFactory).mockRejectedValue(
      new NamedFactoryAPIError(
        "Current factory runtime must be idle before activation.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      ),
    );

    renderWorkstationSelection();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep this draft while the save fails." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Saving failed. Current factory runtime must be idle before activation.",
        ),
      ).toBeTruthy();
    });

    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).value).toBe(
      "Keep this draft while the save fails.",
    );
    expect(
      screen
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("warns in the save confirmation when newer server values would be overwritten", () => {
    const refreshedFactory = buildEditableFactoryDefinition({
      model: "gpt-5.6",
      prompt: "Server changed prompt",
      promptFile: "prompts/server.md",
    });
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    const queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
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

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Keep my local prompt change." },
    });

    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(refreshedFactory),
    );

    rerender(
      <QueryClientProvider client={queryClient}>
        <CurrentSelectionWidget
          currentSelection={buildCurrentSelection({
            selectedNode,
            selection: { kind: "node", nodeId: selectedNode.node_id },
          })}
          now={DETAIL_CARD_NOW}
          selectedWorkExecutionDetails={null}
        />
      </QueryClientProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      screen.getByText(
        "Saving will overwrite newer server values for prompt, model, template with the draft currently shown in the editor.",
      ),
    ).toBeTruthy();
  });

  it("keeps shared-worker model edits disabled while saving workstation-only changes", async () => {
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildSharedWorkerFactoryDefinition()),
    );
    vi.mocked(createFactory).mockResolvedValue(buildSharedWorkerFactoryDefinition({
      prompt: "Updated only the review workstation prompt.",
    }));

    renderWorkstationSelection();

    expect(screen.getByLabelText("Model").getAttribute("disabled")).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated only the review workstation prompt." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Overwrite factory" }));

    await waitFor(() => {
      expect(createFactory).toHaveBeenCalledWith(
        expect.objectContaining({
          workers: [
            expect.objectContaining({
              model: "gpt-5.5",
              name: "processor",
            }),
          ],
          workstations: [
            expect.objectContaining({
              body: "Updated only the review workstation prompt.",
              name: "Review",
            }),
            expect.objectContaining({
              body: "Plan the implementation.",
              name: "Plan",
            }),
          ],
        }),
      );
    });
  });
});

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
}): FactoryValue {
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

function buildSharedWorkerFactoryDefinition(overrides?: { prompt?: string }): FactoryValue {
  return {
    name: "Current Factory",
    workers: [
      {
        model: "gpt-5.5",
        name: "processor",
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
        promptFile: "prompts/review.md",
        worker: "processor",
      },
      {
        body: "Plan the implementation.",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/plan.md",
        worker: "processor",
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

  return renderWithExistingQueryClient(queryClient, view);
}

function renderWithExistingQueryClient(
  queryClient: QueryClient,
  view: ReactNode,
) {
  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}
