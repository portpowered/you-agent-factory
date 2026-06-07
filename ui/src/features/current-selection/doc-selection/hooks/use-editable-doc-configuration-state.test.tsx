import { act, renderHook, waitFor } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableDocConfigurationState } from "./use-editable-doc-configuration-state";

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
    supportingFiles: {
      bundledFiles: [
        {
          content: { encoding: "utf-8", inline: "# Overview\n" },
          targetPath: "factory/docs/overview.md",
          type: "DOC",
        },
      ],
    },
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    workTypes: [],
    ...overrides,
  };
}

describe("useEditableDocConfigurationState", () => {
  const docSelection: DashboardSelection = {
    kind: "doc",
    targetPath: "factory/docs/overview.md",
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

  it("initializes editable doc drafts from the canonical factory definition", async () => {
    const { result } = renderHook(() =>
      useEditableDocConfigurationState(
        docSelection,
        "factory/docs/overview.md",
      ),
    );

    await waitFor(() => {
      expect(result.current).toMatchObject({
        status: "ready",
        draft: {
          fileName: "overview.md",
          inlineContent: "# Overview\n",
        },
        isDirty: false,
        canSave: false,
        hasValidationErrors: false,
        pendingTargetPath: "factory/docs/overview.md",
      });
    });
  });

  it("builds a pending factory definition that updates only the selected doc", async () => {
    const { result } = renderHook(() =>
      useEditableDocConfigurationState(
        docSelection,
        "factory/docs/overview.md",
      ),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable doc state");
      }
      result.current.onInlineContentChange("# Updated\n");
      result.current.onFileNameChange("guide");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      canSave: true,
      isDirty: true,
      pendingTargetPath: "factory/docs/guide.md",
    });
    if (result.current?.status !== "ready") {
      throw new Error("Expected ready editable doc state");
    }
    expect(
      result.current.pendingFactoryDefinition?.supportingFiles?.bundledFiles,
    ).toEqual([
      {
        content: { encoding: "utf-8", inline: "# Updated\n" },
        targetPath: "factory/docs/guide.md",
        type: "DOC",
      },
    ]);
  });
});
