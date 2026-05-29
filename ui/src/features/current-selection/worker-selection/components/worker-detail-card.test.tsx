import { fireEvent, render, screen } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/public";
import type { EditableWorkerConfigurationState } from "../lib/detail-card-types";
import { WorkerDetailCard } from "./worker-detail-card";

vi.mock("../../../current-factory-definition/public", async () => {
  const actual = await vi.importActual(
    "../../../current-factory-definition/public",
  );

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
  };
});

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

  it("renders worker summary and referencing workstations from the factory document", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(<WorkerDetailCard workerName="reviewer" />);

    expect(screen.getByText("reviewer")).toBeTruthy();
    expect(screen.getByText("Model worker")).toBeTruthy();
    expect(screen.getByText("Cursor")).toBeTruthy();
    expect(screen.getByText("gpt-5.5")).toBeTruthy();
    expect(screen.getByText("Script wrap")).toBeTruthy();
    expect(screen.getByText("Review")).toBeTruthy();
    expect(screen.getByText("Plan")).toBeTruthy();
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
        provider: null,
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
      onProviderChange: vi.fn(),
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
    expect(editableConfigurationState.onModelLocalityChange).toHaveBeenCalledWith(
      "LOCAL",
    );
    expect(screen.queryByLabelText("Command")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Collapse worker configuration editor",
      }),
    );
    expect(screen.queryByLabelText("Model provider")).toBeNull();
  });

  it("omits optional model fields when they are absent on the worker", () => {
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

    expect(screen.getByText("Script worker")).toBeTruthy();
    expect(screen.queryByText("Model provider")).toBeNull();
    expect(screen.queryByText("Model")).toBeNull();
    expect(screen.queryByText("Executor provider")).toBeNull();
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
        provider: null,
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
      onProviderChange: vi.fn(),
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

    expect(screen.getByRole("alert").textContent).toContain(
      "Saving reviewer updates every workstation",
    );
    expect(screen.getByRole("button", { name: "Save worker" })).toBeTruthy();
  });

  it("shows scoped save success feedback for the selected worker", () => {
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
            provider: null,
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
          onProviderChange: vi.fn(),
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

    expect(
      screen.getByText(
        /reviewer was updated in the running factory definition/,
      ),
    ).toBeTruthy();
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
        provider: null,
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
      onProviderChange: vi.fn(),
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

    expect(
      screen.getByText(/overwrite newer server values for/i),
    ).toBeTruthy();
    expect(
      screen.getByText(
        /The running factory changed this field while you were editing/i,
      ),
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
        provider: "LINEAR",
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
      onProviderChange: vi.fn(),
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
        provider: null,
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
      onProviderChange: vi.fn(),
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

  it("shows empty referencing workstations copy when none reference the worker", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CURSOR",
            name: "orphan",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [],
      }),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(<WorkerDetailCard workerName="orphan" />);

    expect(
      screen.getByText(
        "No workstations reference this worker in the running factory definition.",
      ),
    ).toBeTruthy();
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
            provider: null,
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
          onProviderChange: vi.fn(),
          onResetToLatest: vi.fn(),
          onTypeChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: buildFactoryDocument(),
          status: "ready",
          validationErrors: {},
        }}
        saveState={{
          message:
            "Current factory definition is stale. Refresh the graph before saving.",
          status: "warning",
        }}
        workerName="reviewer"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Current factory definition is stale",
    );
    expect(
      screen.getByText(
        "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
      ),
    ).toBeTruthy();
  });

  it("shows scoped save error feedback when persistence fails", () => {
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
            provider: null,
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
          onProviderChange: vi.fn(),
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

    expect(screen.getByRole("alert").textContent).toContain("Saving failed.");
    expect(screen.getByRole("alert").textContent).toContain(
      "The running factory could not be saved.",
    );
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
        provider: null,
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
        provider: null,
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
      onProviderChange: vi.fn(),
      markChangesSaved: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: [],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {
        model: "Enter a model before saving this worker.",
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
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByText(
        "Resolve the highlighted fields before saving this worker.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("Enter a model before saving this worker."),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Save worker" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });
});
