// biome-ignore-all lint/style/noExcessiveLinesPerFile lint/complexity/noExcessiveLinesPerFunction: editable worker state regressions share one mocked factory-document seam.
import { act, renderHook, waitFor } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableWorkerConfigurationState as useEditableWorkerConfigurationStateImplementation } from "./use-editable-worker-configuration-state";

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
        model: "gpt-5.5",
        modelProvider: "CODEX",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [{ id: "review", name: "Review", worker: "reviewer" }],
    workTypes: [],
    ...overrides,
  };
}

function useEditableWorkerConfigurationState(
  selection: DashboardSelection | null,
  workerName: string | null,
  locale?: string | null,
) {
  const currentFactoryDocument = useCurrentFactoryDocument(false) as {
    data?: CurrentFactoryDocument;
  };

  return useEditableWorkerConfigurationStateImplementation(
    selection,
    workerName,
    locale,
    currentFactoryDocument.data,
  );
}

describe("useEditableWorkerConfigurationState", () => {
  const workerSelection: DashboardSelection = {
    kind: "worker",
    workerName: "reviewer",
  };

  beforeEach(() => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument(),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);
  });

  it("initializes editable worker draft values from the current factory document", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      draft: {
        model: "gpt-5.5",
        modelProvider: "CODEX",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      isDirty: false,
      validationErrors: {},
    });
  });

  it("blocks save when model provider is cleared", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelProviderChange(null);
      result.current.onModelChange("");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      hasValidationErrors: true,
      isDirty: true,
      validationErrors: {
        modelProvider: expect.any(String),
      },
    });
  });

  it("tracks timeout edits in dirty state", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onTimeoutAmountChange("30");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        timeoutAmount: "30",
        timeoutUnit: "m",
      },
      isDirty: true,
    });
  });

  it("tracks timeout unit edits in dirty state", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onTimeoutAmountChange("1");
      result.current.onTimeoutUnitChange("h");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        timeoutAmount: "1",
        timeoutUnit: "h",
      },
      isDirty: true,
    });
  });

  it("disables save while timeout validation errors are present", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onTimeoutAmountChange("0");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      hasValidationErrors: true,
      isDirty: true,
      validationErrors: {
        timeout: expect.any(String),
      },
    });
  });

  it("tracks stopToken edits in dirty state", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onStopTokenChange("<COMPLETE>");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        stopToken: "<COMPLETE>",
      },
      isDirty: true,
    });
  });

  it("tracks skipPermissions edits in dirty state", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onSkipPermissionsChange(true);
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        skipPermissions: true,
      },
      isDirty: true,
    });
  });

  it("allows save when model is cleared but model provider remains set", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelChange("");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      hasValidationErrors: false,
      isDirty: true,
      validationErrors: {},
    });
  });

  it("builds a pending factory definition that updates only the selected worker", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelProviderChange("CODEX");
      result.current.onModelChange("gpt-5.9");
    });

    expect(result.current?.status).toBe("ready");
    if (result.current?.status !== "ready") {
      return;
    }

    expect(result.current.pendingFactoryDefinition?.workers).toEqual([
      {
        model: "gpt-5.9",
        modelProvider: "CODEX",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ]);
    expect(result.current.isDirty).toBe(true);
    expect(result.current.canSave).toBe(true);
    expect(result.current.hasValidationErrors).toBe(false);
  });

  it("rehydrates clean worker drafts from newer current-factory documents", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.9",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          model: "gpt-5.9",
          modelProvider: "CODEX",
          type: "MODEL_WORKER",
        },
        isDirty: false,
        overwriteFieldNames: [],
        status: "ready",
      });
    });
  });

  it("keeps dirty worker drafts local while tracking newer server-backed field changes", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelChange("Keep this local model draft.");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.9",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          model: "Keep this local model draft.",
          modelProvider: "CODEX",
          type: "MODEL_WORKER",
        },
        isDirty: true,
        overwriteFieldNames: expect.arrayContaining(["model", "modelProvider"]),
        status: "ready",
      });
    });
    if (result.current?.status === "ready") {
      expect(result.current.overwriteFieldNames).toHaveLength(2);
    }
  });

  it("resets dirty worker drafts to the latest server-backed values", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelChange("Keep this local model draft.");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.9",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        isDirty: true,
        overwriteFieldNames: expect.arrayContaining(["model", "modelProvider"]),
        status: "ready",
      });
      if (result.current?.status === "ready") {
        expect(result.current.overwriteFieldNames).toHaveLength(2);
      }
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onResetToLatest();
    });

    expect(result.current).toMatchObject({
      draft: {
        model: "gpt-5.9",
        modelProvider: "CODEX",
        type: "MODEL_WORKER",
      },
      isDirty: false,
      overwriteFieldNames: [],
      status: "ready",
    });
  });

  it("preserves runtime fields in the draft when changing worker type", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "reviewer",
            skipPermissions: true,
            stopToken: "<COMPLETE>",
            timeout: "30m",
            type: "MODEL_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onTypeChange("SCRIPT_WORKER");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      draft: {
        model: "gpt-5.5",
        modelProvider: "CODEX",
        skipPermissions: true,
        stopToken: "<COMPLETE>",
        timeoutAmount: "30",
        timeoutUnit: "m",
        type: "SCRIPT_WORKER",
      },
      isDirty: true,
    });
  });

  it("updates locality, executor, type, and hosted provider draft fields", async () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelLocalityChange("LOCAL");
      result.current.onExecutorProviderChange("SCRIPT_WRAP");
      result.current.onTypeChange("HOSTED_WORKER");
      result.current.onProviderChange("LINEAR");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      draft: {
        executorProvider: "SCRIPT_WRAP",
        modelLocality: "LOCAL",
        provider: "LINEAR",
        type: "HOSTED_WORKER",
      },
      isDirty: true,
    });
  });

  it("markChangesSaved aligns session drafts with the current worker draft", async () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelChange("gpt-5.9");
      result.current.markChangesSaved();
    });

    expect(result.current).toMatchObject({
      status: "ready",
      draft: {
        model: "gpt-5.9",
      },
      isDirty: false,
      overwriteFieldNames: [],
    });
  });

  it("resets the editable worker draft when the selected worker changes", async () => {
    const plannerSelection: DashboardSelection = {
      kind: "worker",
      workerName: "planner",
    };

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
          {
            model: "gpt-4.1",
            modelProvider: "CODEX",
            name: "planner",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [
          { id: "review", name: "Review", worker: "reviewer" },
          { id: "plan", name: "Plan", worker: "planner" },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { rerender, result } = renderHook(
      ({
        currentSelection,
        currentWorkerName,
      }: {
        currentSelection: DashboardSelection;
        currentWorkerName: string;
      }) =>
        useEditableWorkerConfigurationState(
          currentSelection,
          currentWorkerName,
        ),
      {
        initialProps: {
          currentSelection: workerSelection,
          currentWorkerName: "reviewer",
        },
      },
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onModelChange("Keep this local review draft.");
    });

    expect(result.current).toMatchObject({
      draft: {
        model: "Keep this local review draft.",
        modelProvider: "CODEX",
      },
      isDirty: true,
      status: "ready",
    });

    rerender({
      currentSelection: plannerSelection,
      currentWorkerName: "planner",
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          model: "gpt-4.1",
          modelProvider: "CODEX",
          name: "planner",
          type: "MODEL_WORKER",
        },
        isDirty: false,
        overwriteFieldNames: [],
        status: "ready",
      });
    });
  });

  it("blocks save when the worker name duplicates another worker", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
          {
            model: "gpt-4.1",
            modelProvider: "CODEX",
            name: "writer",
            type: "MODEL_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onNameChange("writer");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      hasValidationErrors: true,
      isDirty: true,
      validationErrors: {
        name: expect.stringContaining("writer"),
      },
    });
  });

  it("builds pending hosted Linear poller config for save when required fields are set", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            auth: { secretRef: "secrets/linear-api-key" },
            linear: {
              mapping: { state: "queued", workType: "story" },
              pollInterval: "30s",
              stateIds: ["state-a"],
              teamIds: ["team-a"],
            },
            name: "linear-poller",
            provider: "LINEAR",
            type: "HOSTED_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(
        { kind: "worker", workerName: "linear-poller" },
        "linear-poller",
      ),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onLinearPollIntervalChange("45s");
      result.current.onLinearTeamIdsTextChange("team-a\nteam-b");
      result.current.onLinearMappingWorkTypeChange("task");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      hasValidationErrors: false,
      isDirty: true,
      draft: {
        authSecretRef: "secrets/linear-api-key",
        linearMappingState: "queued",
        linearMappingWorkType: "task",
        linearPollInterval: "45s",
        linearTeamIdsText: "team-a\nteam-b",
        provider: "LINEAR",
        type: "HOSTED_WORKER",
      },
      pendingFactoryDefinition: {
        workers: [
          {
            auth: { secretRef: "secrets/linear-api-key" },
            linear: {
              mapping: { state: "queued", workType: "task" },
              pollInterval: "45s",
              stateIds: ["state-a"],
              teamIds: ["team-a", "team-b"],
            },
            name: "linear-poller",
            provider: "LINEAR",
            type: "HOSTED_WORKER",
          },
        ],
      },
    });
  });

  it("allows save when clearing an existing hosted Linear claim assignee field", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            auth: { secretRef: "secrets/linear-api-key" },
            linear: {
              claim: { assigneeField: "assignee.email" },
              mapping: { state: "queued", workType: "story" },
              pollInterval: "30s",
            },
            name: "linear-poller",
            provider: "LINEAR",
            type: "HOSTED_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(
        { kind: "worker", workerName: "linear-poller" },
        "linear-poller",
      ),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onLinearClaimAssigneeFieldChange("");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      hasValidationErrors: false,
      isDirty: true,
      draft: {
        linearClaimAssigneeField: "",
      },
      pendingFactoryDefinition: {
        workers: [
          {
            auth: { secretRef: "secrets/linear-api-key" },
            linear: {
              mapping: { state: "queued", workType: "story" },
              pollInterval: "30s",
            },
            name: "linear-poller",
            provider: "LINEAR",
            type: "HOSTED_WORKER",
          },
        ],
      },
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition?.workers?.[0]?.linear
        : null,
    ).not.toHaveProperty("claim");
  });

  it("blocks save when hosted Linear poller fields are incomplete", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "linear-poller",
            type: "MODEL_WORKER",
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(
        { kind: "worker", workerName: "linear-poller" },
        "linear-poller",
      ),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onTypeChange("HOSTED_WORKER");
      result.current.onProviderChange("LINEAR");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      hasValidationErrors: true,
      isDirty: true,
      validationErrors: {
        authSecretRef: expect.any(String),
        linearMappingState: expect.any(String),
        linearMappingWorkType: expect.any(String),
      },
    });
  });

  it("includes renamed worker and updated workstation references in pending factory", () => {
    const { result } = renderHook(() =>
      useEditableWorkerConfigurationState(workerSelection, "reviewer"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable worker state");
      }
      result.current.onNameChange("senior-reviewer");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        name: "senior-reviewer",
      },
      pendingFactoryDefinition: {
        workers: [
          expect.objectContaining({
            name: "senior-reviewer",
          }),
        ],
        workstations: [
          expect.objectContaining({
            name: "Review",
            worker: "senior-reviewer",
          }),
        ],
      },
    });
  });
});
