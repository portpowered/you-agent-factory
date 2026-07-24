import { act, renderHook, waitFor } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableDocConfigurationState as useEditableDocConfigurationStateImplementation } from "./use-editable-doc-configuration-state";

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

function useEditableDocConfigurationState(
  selection: DashboardSelection | null,
  targetPath: string | null,
  locale?: string | null,
) {
  const currentFactoryDocument = useCurrentFactoryDocument(false) as {
    data?: CurrentFactoryDocument;
  };

  return useEditableDocConfigurationStateImplementation(
    selection,
    targetPath,
    locale,
    currentFactoryDocument.data,
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: editable doc session regressions stay in one suite.
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

  it("returns undefined for non-doc selections and loading or empty states", async () => {
    expect(
      renderHook(() =>
        useEditableDocConfigurationState(
          { kind: "workstation", workstationName: "review" },
          "factory/docs/overview.md",
        ),
      ).result.current,
    ).toBeUndefined();

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      isError: false,
      isPending: true,
      status: "pending",
    } as never);
    const { result: loadingResult } = renderHook(() =>
      useEditableDocConfigurationState(
        docSelection,
        "factory/docs/overview.md",
      ),
    );
    expect(loadingResult.current).toEqual({ status: "loading" });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        supportingFiles: { bundledFiles: [] },
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);
    const { result: emptyResult } = renderHook(() =>
      useEditableDocConfigurationState(
        docSelection,
        "factory/docs/overview.md",
      ),
    );
    expect(emptyResult.current).toEqual({
      message: "This doc is no longer attached to the current factory.",
      status: "empty",
    });
  });

  it("resets local drafts and tracks overwrite fields when server values change", async () => {
    const { result, rerender } = renderHook(() =>
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
      result.current.onInlineContentChange("# Draft\n");
    });
    expect(result.current).toMatchObject({
      status: "ready",
      isDirty: true,
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument({
        supportingFiles: {
          bundledFiles: [
            {
              content: { encoding: "utf-8", inline: "# Server\n" },
              targetPath: "factory/docs/overview.md",
              type: "DOC",
            },
          ],
        },
      }),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);
    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        status: "ready",
        draft: {
          fileName: "overview.md",
          inlineContent: "# Draft\n",
        },
        overwriteFieldNames: ["inlineContent"],
      });
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("Expected ready editable doc state");
      }
      result.current.onResetToLatest();
      result.current.markChangesSaved();
    });

    expect(result.current).toMatchObject({
      status: "ready",
      isDirty: false,
      overwriteFieldNames: [],
      draft: {
        fileName: "overview.md",
        inlineContent: "# Server\n",
      },
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
