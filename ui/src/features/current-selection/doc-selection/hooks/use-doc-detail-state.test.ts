import { renderHook } from "@testing-library/react";

import * as currentFactoryDefinitionHooks from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useGraphEditorPendingFactoryBridge } from "../../../workflow-activity/state/graph-editor-pending-factory-bridge";
import { useDocDetailState } from "./use-doc-detail-state";

describe("useDocDetailState", () => {
  beforeEach(() => {
    useGraphEditorPendingFactoryBridge.getState().setPendingFactoryDefinition(null);
    vi.restoreAllMocks();
  });

  it("loads doc text from the graph-editor pending factory when the saved document does not list it yet", () => {
    vi.spyOn(
      currentFactoryDefinitionHooks,
      "useCurrentFactoryDocument",
    ).mockReturnValue({
      data: { name: "Current Factory" },
      error: null,
      status: "success",
    } as never);
    useGraphEditorPendingFactoryBridge.getState().setPendingFactoryDefinition({
      name: "Current Factory",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Playbook\n" },
            targetPath: "factory/docs/playbook.md",
            type: "DOC",
          },
        ],
      },
    });

    const { result } = renderHook(() =>
      useDocDetailState("factory/docs/playbook.md"),
    );

    expect(result.current).toEqual({
      status: "ready",
      displayLabel: "playbook.md",
      inlineContent: "# Playbook\n",
      targetPath: "factory/docs/playbook.md",
    });
  });

  it("reports loading, error, empty, and saved-document ready states", () => {
    const useCurrentFactoryDocument = vi.spyOn(
      currentFactoryDefinitionHooks,
      "useCurrentFactoryDocument",
    );

    useCurrentFactoryDocument.mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);
    const { result, rerender } = renderHook(() =>
      useDocDetailState("factory/docs/guide.md"),
    );
    expect(result.current).toEqual({ status: "loading" });

    useCurrentFactoryDocument.mockReturnValue({
      data: undefined,
      error: new Error("network down"),
      status: "error",
    } as never);
    rerender();
    expect(result.current).toEqual({
      status: "error",
      errorMessage: "network down",
    });

    useCurrentFactoryDocument.mockReturnValue({
      data: { name: "Current Factory" },
      error: null,
      status: "success",
    } as never);
    rerender();
    expect(result.current).toEqual({ status: "empty" });

    useCurrentFactoryDocument.mockReturnValue({
      data: {
        name: "Current Factory",
        supportingFiles: {
          bundledFiles: [
            {
              content: { encoding: "utf-8", inline: "# Guide\n" },
              targetPath: "factory/docs/guide.md",
              type: "DOC",
            },
          ],
        },
      },
      error: null,
      status: "success",
    } as never);
    rerender();
    expect(result.current).toEqual({
      status: "ready",
      displayLabel: "guide.md",
      inlineContent: "# Guide\n",
      targetPath: "factory/docs/guide.md",
    });
  });
});
