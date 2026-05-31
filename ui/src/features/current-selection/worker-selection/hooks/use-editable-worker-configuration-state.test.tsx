import { act, renderHook, waitFor } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableWorkerConfigurationState } from "./use-editable-worker-configuration-state";

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
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [{ id: "review", name: "Review", worker: "reviewer" }],
    workTypes: [],
    ...overrides,
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: editable worker state regressions share one mocked factory-document seam.
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
        modelProvider: "CURSOR",
        type: "MODEL_WORKER",
      },
      hasValidationErrors: false,
      isDirty: false,
      validationErrors: {},
    });
  });

  it("blocks save when required model worker fields are cleared", () => {
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
        model: expect.any(String),
        modelProvider: expect.any(String),
      },
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
          modelProvider: "CURSOR",
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
            modelProvider: "CURSOR",
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
        modelProvider: "CURSOR",
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
          type: "MODEL_WORKER",
        },
        isDirty: false,
        overwriteFieldNames: [],
        status: "ready",
      });
    });
  });
});
