import { act, renderHook } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/public";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableWorkerConfigurationState } from "./use-editable-worker-configuration-state";

vi.mock("../../../current-factory-definition/public", async () => {
  const actual = await vi.importActual("../../../current-factory-definition/public");

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
  };
});

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
      draft: {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        type: "MODEL_WORKER",
      },
      isDirty: false,
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
  });
});
