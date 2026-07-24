// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: editable work type state regressions share one mocked factory-document seam.
import { act, renderHook, waitFor } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableWorkTypeConfigurationState as useEditableWorkTypeConfigurationStateImplementation } from "./use-editable-work-type-configuration-state";

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
    workstations: [
      {
        id: "review",
        inputs: [],
        name: "Review",
        worker: "reviewer",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
      {
        name: "bug",
        states: [{ name: "open", type: "INITIAL" }],
      },
    ],
    ...overrides,
  };
}

function useEditableWorkTypeConfigurationState(
  selection: DashboardSelection | null,
  workTypeName: string | null,
  locale?: string | null,
) {
  const currentFactoryDocument = useCurrentFactoryDocument(false) as {
    data?: CurrentFactoryDocument;
  };

  return useEditableWorkTypeConfigurationStateImplementation(
    selection,
    workTypeName,
    locale,
    currentFactoryDocument.data,
  );
}

describe("useEditableWorkTypeConfigurationState", () => {
  const storySelection: DashboardSelection = {
    kind: "work-type",
    workTypeName: "story",
  };
  const bugSelection: DashboardSelection = {
    kind: "work-type",
    workTypeName: "bug",
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

  it("returns undefined when selection is not a work type", () => {
    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(
        { kind: "worker", workerName: "reviewer" },
        null,
      ),
    );

    expect(result.current).toBeUndefined();
  });

  it("initializes editable work type draft values from the current factory document", () => {
    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(storySelection, "story"),
    );

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      draft: {
        handlingBehavior: null,
        name: "story",
      },
      hasValidationErrors: false,
      isDirty: false,
    });
  });

  it("marks the session dirty when the work type name changes", () => {
    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(storySelection, "story"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work type state");
      }
      result.current.onNameChange("feature");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        name: "feature",
      },
      isDirty: true,
    });
  });

  it("transfers default handling when marking a different work type as default", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            name: "story",
            states: [{ name: "queued", type: "INITIAL" }],
          },
          {
            name: "bug",
            states: [{ name: "open", type: "INITIAL" }],
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(bugSelection, "bug"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work type state");
      }
      result.current.onHandlingBehaviorChange(["DEFAULT"]);
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      hasValidationErrors: false,
      isDirty: true,
      draft: {
        handlingBehavior: ["DEFAULT"],
        name: "bug",
      },
    });

    if (result.current?.status !== "ready") {
      return;
    }

    expect(result.current.pendingFactoryDefinition?.workTypes).toEqual([
      {
        name: "story",
        states: [{ name: "queued", type: "INITIAL" }],
      },
      {
        handlingBehavior: ["DEFAULT"],
        name: "bug",
        states: [{ name: "open", type: "INITIAL" }],
      },
    ]);
  });

  it("marks the session dirty when handlingBehavior changes", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            name: "story",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(storySelection, "story"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work type state");
      }
      result.current.onHandlingBehaviorChange(null);
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        handlingBehavior: null,
        name: "story",
      },
      hasValidationErrors: false,
      isDirty: true,
    });
  });

  it("builds a pending factory definition that updates only the selected work type", () => {
    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(storySelection, "story"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work type state");
      }
      result.current.onNameChange("feature");
    });

    expect(result.current?.status).toBe("ready");
    if (result.current?.status !== "ready") {
      return;
    }

    expect(result.current.pendingFactoryDefinition?.workTypes).toEqual([
      {
        name: "feature",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
      {
        name: "bug",
        states: [{ name: "open", type: "INITIAL" }],
      },
    ]);
  });

  it("clears dirty state after markChangesSaved", () => {
    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(storySelection, "story"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work type state");
      }
      result.current.onNameChange("feature");
      result.current.markChangesSaved();
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      isDirty: false,
    });
  });

  it("resets the draft when the selected work type changes", async () => {
    const { rerender, result } = renderHook(
      ({
        currentSelection,
        currentWorkTypeName,
      }: {
        currentSelection: DashboardSelection;
        currentWorkTypeName: string;
      }) =>
        useEditableWorkTypeConfigurationState(
          currentSelection,
          currentWorkTypeName,
        ),
      {
        initialProps: {
          currentSelection: storySelection,
          currentWorkTypeName: "story",
        },
      },
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work type state");
      }
      result.current.onNameChange("Keep this local story draft.");
    });

    expect(result.current).toMatchObject({
      draft: {
        name: "Keep this local story draft.",
      },
      isDirty: true,
      status: "ready",
    });

    rerender({
      currentSelection: bugSelection,
      currentWorkTypeName: "bug",
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          handlingBehavior: null,
          name: "bug",
        },
        isDirty: false,
        status: "ready",
      });
    });
  });

  it("reports loading while the event-backed factory document is unavailable", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      isError: false,
      isPending: true,
      status: "pending",
    } as never);

    const { result: loadingResult } = renderHook(() =>
      useEditableWorkTypeConfigurationState(storySelection, "story"),
    );
    expect(loadingResult.current).toEqual({ status: "loading" });
  });

  it("returns empty when the selected work type is missing from the factory document", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({ workTypes: [] }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkTypeConfigurationState(storySelection, "story"),
    );

    expect(result.current).toMatchObject({
      status: "empty",
      message: expect.stringContaining("work type"),
    });
  });
});
