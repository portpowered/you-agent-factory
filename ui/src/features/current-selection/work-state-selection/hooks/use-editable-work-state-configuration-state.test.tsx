// biome-ignore-all lint/style/noExcessiveLinesPerFile lint/complexity/noExcessiveLinesPerFunction: editable work-state hook regressions share one mocked factory-document seam.
import { act, renderHook, waitFor } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableWorkStateConfigurationState as useEditableWorkStateConfigurationStateImplementation } from "./use-editable-work-state-configuration-state";

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
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
    ],
    ...overrides,
  };
}

function useEditableWorkStateConfigurationState(
  selection: DashboardSelection | null,
  placeId: string | null,
  locale?: string | null,
) {
  const currentFactoryDocument = useCurrentFactoryDocument(false) as {
    data?: CurrentFactoryDocument;
  };

  return useEditableWorkStateConfigurationStateImplementation(
    selection,
    placeId,
    locale,
    currentFactoryDocument.data,
  );
}

describe("useEditableWorkStateConfigurationState", () => {
  const queuedSelection: DashboardSelection = {
    kind: "state-node",
    placeId: "story:queued",
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

  it("initializes editable work state draft values from the current factory document", () => {
    const { result } = renderHook(() =>
      useEditableWorkStateConfigurationState(queuedSelection, "story:queued"),
    );

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      draft: {
        name: "queued",
        type: "INITIAL",
      },
      hasValidationErrors: false,
      isDirty: false,
      originalStateName: "queued",
      validationErrors: {},
      workTypeName: "story",
    });
  });

  it("detects dirty state when the work state name changes", () => {
    const { result } = renderHook(() =>
      useEditableWorkStateConfigurationState(queuedSelection, "story:queued"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work state");
      }
      result.current.onNameChange("ready");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      draft: {
        name: "ready",
        type: "INITIAL",
      },
      isDirty: true,
    });
  });

  it("blocks save when the work state name duplicates another state in the work type", () => {
    const { result } = renderHook(() =>
      useEditableWorkStateConfigurationState(queuedSelection, "story:queued"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work state");
      }
      result.current.onNameChange("done");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      hasValidationErrors: true,
      isDirty: true,
      validationErrors: {
        name: expect.stringContaining("done"),
      },
    });
  });

  it("builds a pending factory definition that renames the selected work state", () => {
    const { result } = renderHook(() =>
      useEditableWorkStateConfigurationState(queuedSelection, "story:queued"),
    );

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work state");
      }
      result.current.onNameChange("ready");
    });

    expect(result.current?.status).toBe("ready");
    if (result.current?.status !== "ready") {
      return;
    }

    expect(
      result.current.pendingFactoryDefinition?.workTypes?.[0]?.states,
    ).toEqual([
      { name: "ready", type: "INITIAL" },
      { name: "done", type: "TERMINAL" },
    ]);
  });

  it("markChangesSaved clears dirty state for the current draft", async () => {
    const { result } = renderHook(() =>
      useEditableWorkStateConfigurationState(queuedSelection, "story:queued"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work state");
      }
      result.current.onNameChange("ready");
      result.current.markChangesSaved();
    });

    expect(result.current).toMatchObject({
      status: "ready",
      draft: {
        name: "ready",
        type: "INITIAL",
      },
      isDirty: false,
    });
  });

  it("resets the editable work state draft when the selected place changes", async () => {
    const doneSelection: DashboardSelection = {
      kind: "state-node",
      placeId: "story:done",
    };

    const { rerender, result } = renderHook(
      ({
        currentSelection,
        currentPlaceId,
      }: {
        currentSelection: DashboardSelection;
        currentPlaceId: string;
      }) =>
        useEditableWorkStateConfigurationState(
          currentSelection,
          currentPlaceId,
        ),
      {
        initialProps: {
          currentPlaceId: "story:queued",
          currentSelection: queuedSelection,
        },
      },
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable work state");
      }
      result.current.onNameChange("Keep this local queued draft.");
    });

    expect(result.current).toMatchObject({
      draft: {
        name: "Keep this local queued draft.",
        type: "INITIAL",
      },
      isDirty: true,
      status: "ready",
    });

    rerender({
      currentPlaceId: "story:done",
      currentSelection: doneSelection,
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          name: "done",
          type: "TERMINAL",
        },
        isDirty: false,
        originalStateName: "done",
        status: "ready",
        workTypeName: "story",
      });
    });
  });

  it("returns undefined when the selection is not a state node", () => {
    const { result } = renderHook(() =>
      useEditableWorkStateConfigurationState(
        { kind: "worker", workerName: "reviewer" },
        "story:queued",
      ),
    );

    expect(result.current).toBeUndefined();
  });
});
