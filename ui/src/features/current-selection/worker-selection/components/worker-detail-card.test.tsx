import { fireEvent, render, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { expectNoInlineSaveOutcomesIn } from "../../base/components/current-selection-save-toast-test-helpers";
import type {
  EditableWorkerConfigurationState,
  EditableWorkerSaveState,
} from "../lib/detail-card-types";
import { WorkerDetailCard } from "./worker-detail-card";
import { EditableWorkerConfigurationHeaderActions } from "./worker-save-controls";

const CURRENT_SELECTION_FORM_FIELDS_SELECTOR = ".grid.grid-cols-1.gap-3";

vi.mock(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

function mockFactoryDocumentQuery(
  overrides: Partial<ReturnType<typeof useCurrentFactoryDocument>> = {},
) {
  vi.mocked(useCurrentFactoryDocument).mockReturnValue({
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
    ...overrides,
  } as never);
}

function workerDetailHeaderActionSection() {
  const card = screen.getByRole("article", { name: "Current selection" });
  const undoButton = within(card).getByRole("button", {
    name: "Undo selection",
  });
  const actionSection = undoButton.closest(
    "[data-dashboard-action-row-section='actions']",
  );
  if (!actionSection) {
    throw new Error("expected header action section");
  }

  return actionSection as HTMLElement;
}

function editableConfigurationSection() {
  const heading = screen.getByRole("heading", {
    name: "Worker configuration",
  });
  const section = heading.closest("section");
  if (!section) {
    throw new Error("expected editable configuration section");
  }

  return section;
}

function buildWorkerHeaderActions({
  canDiscard = false,
  canSave,
  onDiscard = vi.fn(),
  onSave = vi.fn(),
  saveState = { status: "idle" },
}: {
  canDiscard?: boolean;
  canSave: boolean;
  onDiscard?: () => void;
  onSave?: () => void;
  saveState?: EditableWorkerSaveState;
}) {
  return (
    <EditableWorkerConfigurationHeaderActions
      canDiscard={canDiscard}
      canSave={canSave}
      onDiscard={onDiscard}
      onSave={onSave}
      saveState={saveState}
    />
  );
}

function buildFactoryDocument(
  overrides?: Partial<CurrentFactoryDocument>,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    workers: [
      {
        executorProvider: "SCRIPT_WRAP",
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        id: "review",
        name: "Review",
        worker: "reviewer",
      },
      {
        id: "plan",
        name: "Plan",
        worker: "reviewer",
      },
    ],
    workTypes: [],
    ...overrides,
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: WorkerDetailCard coverage keeps loading, ready, and editable state regressions together.
describe("WorkerDetailCard", () => {
  beforeEach(() => {
    mockFactoryDocumentQuery();
  });

  it("shows loading state while the current factory document is pending", () => {
    render(<WorkerDetailCard workerName="reviewer" />);

    expect(
      screen.getByText(
        "Loading the current factory definition for this worker.",
      ),
    ).toBeTruthy();
  });

  it("shows error state when the current factory document fails to load", () => {
    mockFactoryDocumentQuery({
      error: { message: "Factory unavailable" },
      isError: true,
      isPending: false,
      status: "error",
    } as never);

    render(<WorkerDetailCard workerName="reviewer" />);

    expect(screen.getByRole("alert").textContent).toContain(
      "Worker definition unavailable.",
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "Factory unavailable",
    );
  });

  it("shows empty state when the selected worker is missing from the factory document", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(<WorkerDetailCard workerName="missing-worker" />);

    expect(
      screen.getByText(
        "This running factory definition does not include the selected worker.",
      ),
    ).toBeTruthy();
  });

  it("omits summary and referencing-workstations sections from the worker detail panel", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(<WorkerDetailCard workerName="reviewer" />);

    expect(screen.getByText("reviewer")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Worker configuration" }),
    ).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Summary" })).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Referencing workstations" }),
    ).toBeNull();
    expect(screen.queryByText("Review")).toBeNull();
    expect(screen.queryByText("Plan")).toBeNull();
  });

  it("renders editable worker fields for model workers", () => {
    const onModelProviderChange = vi.fn();
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: false,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review"],
      },
      isDirty: false,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange,
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        workerName="reviewer"
      />,
    );

    fireEvent.change(screen.getByLabelText("Model provider"), {
      target: { value: "CODEX" },
    });
    fireEvent.change(screen.getByLabelText("Model locality"), {
      target: { value: "LOCAL" },
    });

    expect(onModelProviderChange).toHaveBeenCalledWith("CODEX");
    expect(
      editableConfigurationState.onModelLocalityChange,
    ).toHaveBeenCalledWith("LOCAL");
    expect(screen.getByLabelText("Worker name")).toHaveProperty(
      "value",
      "reviewer",
    );
    expect(screen.queryByLabelText("Command")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Collapse worker configuration editor",
      }),
    );
    expect(screen.queryByLabelText("Model provider")).toBeNull();
  });

  it("renders model worker configuration without a runner capability matrix", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: false,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review"],
      },
      isDirty: false,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        workerName="reviewer"
      />,
    );

    const configurationHeading = screen.getByRole("heading", {
      name: "Worker configuration",
    });
    const configurationSection = configurationHeading.closest("section");
    expect(configurationSection).toBeTruthy();

    const configuration = within(configurationSection as HTMLElement);
    expect(configuration.getByLabelText("Model provider")).toBeTruthy();
    expect(configuration.getByLabelText("Model")).toBeTruthy();
    expect(configuration.getByLabelText("Executor provider")).toBeTruthy();
    expect(configuration.queryByText("Runner capability support")).toBeNull();
    expect(configuration.queryByText("Supported")).toBeNull();
    expect(configuration.queryByText("Unsupported")).toBeNull();
  });

  it("does not list referencing workstations outside the editable configuration section", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument({
        workers: [{ name: "script-runner", type: "SCRIPT_WORKER" }],
        workstations: [{ id: "run", name: "Run", worker: "script-runner" }],
      }),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(<WorkerDetailCard workerName="script-runner" />);

    expect(
      screen.queryByRole("heading", { name: "Referencing workstations" }),
    ).toBeNull();
    expect(screen.queryByText("Run")).toBeNull();
  });

  it("warns when saving would affect multiple referencing workstations", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: true,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review", "Plan"],
      },
      isDirty: true,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkerHeaderActions({
          canDiscard: true,
          canSave: true,
        })}
        workerName="reviewer"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Saving reviewer updates workstations",
    );

    const headerActions = workerDetailHeaderActionSection();
    expect(
      within(headerActions).getAllByRole("button", { name: "Save worker" }),
    ).toHaveLength(1);
    expect(
      within(headerActions).getByRole("button", {
        name: "Discard local changes",
      }),
    ).toBeTruthy();
    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Save worker",
      }),
    ).toBeNull();
    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Discard local changes",
      }),
    ).toBeNull();
  });

  it("stacks configuration fields vertically and keeps save and discard in the header only", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: true,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review"],
      },
      isDirty: true,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    const onSave = vi.fn();
    const onDiscard = vi.fn();

    const { container } = render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkerHeaderActions({
          canDiscard: true,
          canSave: true,
          onDiscard,
          onSave,
        })}
        workerName="reviewer"
      />,
    );

    const fieldGroup = container.querySelector(
      CURRENT_SELECTION_FORM_FIELDS_SELECTOR,
    );
    expect(fieldGroup).not.toBeNull();
    expect(fieldGroup?.className).not.toMatch(/md:grid-cols-\d/);
    expect(fieldGroup?.className).not.toMatch(/xl:grid-cols-\d/);

    const headerActions = workerDetailHeaderActionSection();
    const saveButtons = within(headerActions).getAllByRole("button", {
      name: "Save worker",
    });
    const discardButtons = within(headerActions).getAllByRole("button", {
      name: "Discard local changes",
    });
    expect(saveButtons).toHaveLength(1);
    expect(discardButtons).toHaveLength(1);

    fireEvent.click(saveButtons[0]);
    expect(onSave).toHaveBeenCalledTimes(1);

    fireEvent.click(discardButtons[0]);
    expect(onDiscard).toHaveBeenCalledTimes(1);

    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Save worker",
      }),
    ).toBeNull();
    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Discard local changes",
      }),
    ).toBeNull();
  });

  it("does not render inline save success feedback for the selected worker", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={{
          canSave: false,
          draft: {
            argsText: "",
            body: "",
            command: "",
            executorProvider: null,
            model: "gpt-5.5",
            modelLocality: null,
            modelProvider: "CURSOR",
            name: "reviewer",
            provider: null,
            skipPermissions: false,
            stopToken: "",
            timeoutAmount: "",
            timeoutUnit: "m",
            type: "MODEL_WORKER",
          },
          hasValidationErrors: false,
          initialValues: {
            args: [],
            body: null,
            command: null,
            executorProvider: null,
            model: "gpt-5.5",
            modelLocality: null,
            modelProvider: "CURSOR",
            provider: null,
            skipPermissions: null,
            stopToken: null,
            timeout: null,
            type: "MODEL_WORKER",
            workerName: "reviewer",
            workstationNames: ["Review"],
          },
          isDirty: false,
          markChangesSaved: vi.fn(),
          onArgsTextChange: vi.fn(),
          onBodyChange: vi.fn(),
          onCommandChange: vi.fn(),
          onExecutorProviderChange: vi.fn(),
          onModelChange: vi.fn(),
          onModelLocalityChange: vi.fn(),
          onModelProviderChange: vi.fn(),
          onNameChange: vi.fn(),
          onProviderChange: vi.fn(),
          onSkipPermissionsChange: vi.fn(),
          onStopTokenChange: vi.fn(),
          onTimeoutAmountChange: vi.fn(),
          onTimeoutUnitChange: vi.fn(),
          onResetToLatest: vi.fn(),
          onTypeChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {},
        }}
        saveState={{ status: "success" }}
        workerName="reviewer"
      />,
    );

    expectNoInlineSaveOutcomesIn(
      screen
        .getByRole("heading", { name: "Worker configuration" })
        .closest("section") ?? document.body,
    );
  });

  it("shows overwrite warning and server-changed hints for dirty worker drafts", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: true,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CODEX",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review"],
      },
      isDirty: true,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: ["modelProvider"],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        workerName="reviewer"
      />,
    );

    expect(screen.getByText(/overwrite newer server values for/i)).toBeTruthy();
    expect(
      screen.getByText(
        /The running factory changed this field while you were editing/i,
      ),
    ).toBeTruthy();
  });

  it("omits global unsaved helper paragraphs for dirty ready-state worker drafts", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: true,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review"],
      },
      isDirty: true,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkerHeaderActions({
          canDiscard: true,
          canSave: true,
        })}
        workerName="reviewer"
      />,
    );

    expect(
      screen.queryByText("You have unsaved changes for this worker."),
    ).toBeNull();
    expect(
      screen.queryByText(
        "Changes stay local to this edit session until you save the running factory.",
      ),
    ).toBeNull();
    expect(
      screen.getAllByRole("button", { name: "Save worker" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: "Discard local changes" }),
    ).toBeTruthy();
  });

  it("renders hosted worker provider fields in the editable configuration section", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: false,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "",
        modelLocality: null,
        modelProvider: null,
        name: "linear-bot",
        provider: "LINEAR",
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "HOSTED_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: null,
        modelLocality: null,
        modelProvider: null,
        provider: "LINEAR",
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "HOSTED_WORKER",
        workerName: "linear-bot",
        workstationNames: ["Sync"],
      },
      isDirty: false,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument({
        workers: [
          {
            name: "linear-bot",
            provider: "LINEAR",
            type: "HOSTED_WORKER",
          },
        ],
        workstations: [{ id: "sync", name: "Sync", worker: "linear-bot" }],
      }),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        workerName="linear-bot"
      />,
    );

    expect(screen.getByLabelText("Hosted provider")).toBeTruthy();
  });

  it("renders script worker command and body fields in the editable configuration section", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: false,
      draft: {
        argsText: "check\nlint",
        body: "Run the check",
        command: "make check",
        executorProvider: null,
        model: "",
        modelLocality: null,
        modelProvider: null,
        name: "script-runner",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "SCRIPT_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: ["check", "lint"],
        body: "Run the check",
        command: "make check",
        executorProvider: null,
        model: null,
        modelLocality: null,
        modelProvider: null,
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "SCRIPT_WORKER",
        workerName: "script-runner",
        workstationNames: ["Run"],
      },
      isDirty: false,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument({
        workers: [
          {
            body: "Run the check",
            command: "make check",
            name: "script-runner",
            type: "SCRIPT_WORKER",
          },
        ],
        workstations: [{ id: "run", name: "Run", worker: "script-runner" }],
      }),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        workerName="script-runner"
      />,
    );

    expect(screen.getByLabelText("Command")).toBeTruthy();
    expect(screen.getByLabelText("Body")).toBeTruthy();
    expect(screen.getByLabelText("Args")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Command"), {
      target: { value: "make test" },
    });
    fireEvent.change(screen.getByLabelText("Args"), {
      target: { value: "unit" },
    });
    fireEvent.change(screen.getByLabelText("Body"), {
      target: { value: "Run unit tests" },
    });

    expect(editableConfigurationState.onCommandChange).toHaveBeenCalledWith(
      "make test",
    );
    expect(editableConfigurationState.onArgsTextChange).toHaveBeenCalledWith(
      "unit",
    );
    expect(editableConfigurationState.onBodyChange).toHaveBeenCalledWith(
      "Run unit tests",
    );
  });

  it("shows stale-version save warning without discarding the worker draft", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={{
          canSave: true,
          draft: {
            argsText: "",
            body: "",
            command: "",
            executorProvider: null,
            model: "gpt-5.5",
            modelLocality: null,
            modelProvider: "CURSOR",
            name: "reviewer",
            provider: null,
            skipPermissions: false,
            stopToken: "",
            timeoutAmount: "",
            timeoutUnit: "m",
            type: "MODEL_WORKER",
          },
          hasValidationErrors: false,
          initialValues: {
            args: [],
            body: null,
            command: null,
            executorProvider: null,
            model: "gpt-5.5",
            modelLocality: null,
            modelProvider: "CURSOR",
            provider: null,
            skipPermissions: null,
            stopToken: null,
            timeout: null,
            type: "MODEL_WORKER",
            workerName: "reviewer",
            workstationNames: ["Review"],
          },
          isDirty: true,
          markChangesSaved: vi.fn(),
          onArgsTextChange: vi.fn(),
          onBodyChange: vi.fn(),
          onCommandChange: vi.fn(),
          onExecutorProviderChange: vi.fn(),
          onModelChange: vi.fn(),
          onModelLocalityChange: vi.fn(),
          onModelProviderChange: vi.fn(),
          onNameChange: vi.fn(),
          onProviderChange: vi.fn(),
          onSkipPermissionsChange: vi.fn(),
          onStopTokenChange: vi.fn(),
          onTimeoutAmountChange: vi.fn(),
          onTimeoutUnitChange: vi.fn(),
          onResetToLatest: vi.fn(),
          onTypeChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {},
        }}
        saveState={{
          message:
            "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
          status: "warning",
        }}
        workerName="reviewer"
      />,
    );

    expect(
      screen.queryByText(/Current factory definition is stale/),
    ).toBeNull();
    expect(
      screen.queryByText(
        "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
      ),
    ).toBeNull();
  });

  it("does not render inline save error feedback when persistence fails", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={{
          canSave: true,
          draft: {
            argsText: "",
            body: "",
            command: "",
            executorProvider: null,
            model: "gpt-5.5",
            modelLocality: null,
            modelProvider: "CURSOR",
            name: "reviewer",
            provider: null,
            skipPermissions: false,
            stopToken: "",
            timeoutAmount: "",
            timeoutUnit: "m",
            type: "MODEL_WORKER",
          },
          hasValidationErrors: false,
          initialValues: {
            args: [],
            body: null,
            command: null,
            executorProvider: null,
            model: "gpt-5.5",
            modelLocality: null,
            modelProvider: "CURSOR",
            provider: null,
            skipPermissions: null,
            stopToken: null,
            timeout: null,
            type: "MODEL_WORKER",
            workerName: "reviewer",
            workstationNames: ["Review"],
          },
          isDirty: true,
          markChangesSaved: vi.fn(),
          onArgsTextChange: vi.fn(),
          onBodyChange: vi.fn(),
          onCommandChange: vi.fn(),
          onExecutorProviderChange: vi.fn(),
          onModelChange: vi.fn(),
          onModelLocalityChange: vi.fn(),
          onModelProviderChange: vi.fn(),
          onNameChange: vi.fn(),
          onProviderChange: vi.fn(),
          onSkipPermissionsChange: vi.fn(),
          onStopTokenChange: vi.fn(),
          onTimeoutAmountChange: vi.fn(),
          onTimeoutUnitChange: vi.fn(),
          onResetToLatest: vi.fn(),
          onTypeChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {},
        }}
        saveState={{
          errorMessage: "The running factory could not be saved.",
          status: "error",
        }}
        workerName="reviewer"
      />,
    );

    expectNoInlineSaveOutcomesIn(
      screen
        .getByRole("heading", { name: "Worker configuration" })
        .closest("section") ?? document.body,
    );
  });

  it("shows model worker field help and allows save with provider only", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: true,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review"],
      },
      isDirty: true,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkerHeaderActions({ canSave: true })}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByText(
        "Required for model workers; sets routing and default model.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("Blank uses the provider default model."),
    ).toBeTruthy();
    expect(
      screen.queryByText("Enter a model before saving this worker."),
    ).toBeNull();
    const headerSave = within(workerDetailHeaderActionSection()).getByRole(
      "button",
      { name: "Save worker" },
    );
    expect(headerSave.hasAttribute("disabled")).toBe(false);
  });

  it("disables save and shows field errors while validation is unresolved", () => {
    const editableConfigurationState: EditableWorkerConfigurationState = {
      canSave: false,
      draft: {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "",
        modelLocality: null,
        modelProvider: null,
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: true,
      initialValues: {
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review"],
      },
      isDirty: true,
      onArgsTextChange: vi.fn(),
      onBodyChange: vi.fn(),
      onCommandChange: vi.fn(),
      onExecutorProviderChange: vi.fn(),
      onModelChange: vi.fn(),
      onModelLocalityChange: vi.fn(),
      onModelProviderChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onSkipPermissionsChange: vi.fn(),
      onStopTokenChange: vi.fn(),
      onTimeoutAmountChange: vi.fn(),
      onTimeoutUnitChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {
        modelProvider: "Select a model provider before saving this worker.",
      },
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <WorkerDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkerHeaderActions({ canSave: false })}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByText(
        "Resolve the highlighted fields before saving this worker.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("Select a model provider before saving this worker."),
    ).toBeTruthy();
    expect(
      screen.queryByText("Enter a model before saving this worker."),
    ).toBeNull();
    expect(
      within(workerDetailHeaderActionSection())
        .getByRole("button", { name: "Save worker" })
        .hasAttribute("disabled"),
    ).toBe(true);
    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Save worker",
      }),
    ).toBeNull();
  });
});
