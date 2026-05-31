// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: editable resource state regressions share one mocked factory-document seam.
import { act, renderHook, waitFor } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableResourceConfigurationState } from "./use-editable-resource-configuration-state";

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
    resources: [
      {
        capacity: 2,
        name: "agent-slot",
        type: "INVOCATION_SLOT",
      },
      {
        capacity: 1,
        name: "voice-model",
        type: "MODEL",
      },
    ],
    workTypes: [],
    ...overrides,
  };
}

describe("useEditableResourceConfigurationState", () => {
  const resourceSelection: DashboardSelection = {
    kind: "resource",
    resourceName: "agent-slot",
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

  it("initializes editable resource drafts from the canonical factory definition", async () => {
    const { result } = renderHook(() =>
      useEditableResourceConfigurationState(resourceSelection, "agent-slot"),
    );

    await waitFor(() => {
      expect(result.current).toMatchObject({
        status: "ready",
        draft: {
          capacityText: "2",
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
        isDirty: false,
        canSave: false,
        hasValidationErrors: false,
      });
    });
  });

  it("builds a pending factory definition that updates only the selected resource", async () => {
    const { result } = renderHook(() =>
      useEditableResourceConfigurationState(resourceSelection, "agent-slot"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable resource state");
      }
      result.current.onCapacityChange("4");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      hasValidationErrors: false,
      isDirty: true,
    });
    if (result.current?.status !== "ready") {
      return;
    }

    expect(result.current.pendingFactoryDefinition?.resources).toEqual([
      {
        capacity: 4,
        name: "agent-slot",
        type: "INVOCATION_SLOT",
      },
      {
        capacity: 1,
        name: "voice-model",
        type: "MODEL",
      },
    ]);
  });

  it("blocks save for rename collisions and invalid capacity", async () => {
    const { result } = renderHook(() =>
      useEditableResourceConfigurationState(resourceSelection, "agent-slot"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable resource state");
      }
      result.current.onNameChange("voice-model");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      hasValidationErrors: true,
      isDirty: true,
    });
    if (result.current?.status !== "ready") {
      return;
    }
    expect(result.current.validationErrors.name).toContain("voice-model");

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable resource state");
      }
      result.current.onNameChange("agent-slot");
      result.current.onCapacityChange("0");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: false,
      hasValidationErrors: true,
    });
    if (result.current?.status !== "ready") {
      return;
    }
    expect(result.current.validationErrors.capacity).toBeTruthy();
  });

  it("flags overwrite fields when the server-backed factory document changes during editing", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableResourceConfigurationState(resourceSelection, "agent-slot"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable resource state");
      }
      result.current.onCapacityChange("9");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        resources: [
          {
            capacity: 4,
            name: "agent-slot",
            type: "INVOCATION_SLOT",
          },
          {
            capacity: 1,
            name: "voice-model",
            type: "MODEL",
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
          capacityText: "9",
          name: "agent-slot",
        },
        isDirty: true,
        overwriteFieldNames: ["capacity"],
        status: "ready",
      });
    });
  });

  it("resets dirty resource drafts to the latest server-backed values", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableResourceConfigurationState(resourceSelection, "agent-slot"),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable resource state");
      }
      result.current.onCapacityChange("9");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        resources: [
          {
            capacity: 4,
            name: "agent-slot",
            type: "INVOCATION_SLOT",
          },
          {
            capacity: 1,
            name: "voice-model",
            type: "MODEL",
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
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable resource state");
      }
      result.current.onResetToLatest();
    });

    expect(result.current).toMatchObject({
      draft: {
        capacityText: "4",
        name: "agent-slot",
      },
      isDirty: false,
      overwriteFieldNames: [],
      status: "ready",
    });
  });
});
