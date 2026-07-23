import { act, renderHook } from "@testing-library/react";

import { useGraphEditorPendingFactoryBridge } from "../../../workflow-activity/state/graph-editor-pending-factory-bridge";
import { useDocDetailState } from "./use-doc-detail-state";

describe("useDocDetailState", () => {
  beforeEach(() => {
    act(() => {
      useGraphEditorPendingFactoryBridge
        .getState()
        .setPendingFactoryDefinition(null);
    });
  });

  afterEach(() => {
    act(() => {
      useGraphEditorPendingFactoryBridge
        .getState()
        .setPendingFactoryDefinition(null);
    });
  });

  it("loads nested doc text from the graph-editor pending factory", () => {
    const nestedDocPath = "factory/docs/standards/review.md";
    act(() => {
      useGraphEditorPendingFactoryBridge
        .getState()
        .setPendingFactoryDefinition({
          name: "Current Factory",
          supportingFiles: {
            bundledFiles: [
              {
                content: { encoding: "utf-8", inline: "# Review standards\n" },
                targetPath: nestedDocPath,
                type: "DOC",
              },
            ],
          },
        });
    });

    const { result } = renderHook(() =>
      useDocDetailState({ targetPath: nestedDocPath }),
    );

    expect(result.current).toEqual({
      status: "ready",
      displayLabel: "review.md",
      inlineContent: "# Review standards\n",
      targetPath: nestedDocPath,
    });
  });

  it("loads doc text from the graph-editor pending factory when the saved event factory does not list it yet", () => {
    act(() => {
      useGraphEditorPendingFactoryBridge
        .getState()
        .setPendingFactoryDefinition({
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
    });

    const { result } = renderHook(() =>
      useDocDetailState({ targetPath: "factory/docs/playbook.md" }),
    );

    expect(result.current).toEqual({
      status: "ready",
      displayLabel: "playbook.md",
      inlineContent: "# Playbook\n",
      targetPath: "factory/docs/playbook.md",
    });
  });

  it("reports empty and saved-event-factory ready states", () => {
    const { result, rerender } = renderHook(
      ({
        savedBundledDoc,
      }: {
        savedBundledDoc?: Parameters<
          typeof useDocDetailState
        >[0]["savedBundledDoc"];
      }) =>
        useDocDetailState({
          savedBundledDoc,
          targetPath: "factory/docs/guide.md",
        }),
      {
        initialProps: {},
      },
    );
    expect(result.current).toEqual({ status: "empty" });

    rerender({
      savedBundledDoc: {
        content: { encoding: "utf-8", inline: "# Guide\n" },
        targetPath: "factory/docs/guide.md",
        type: "DOC",
      },
    });
    expect(result.current).toEqual({
      status: "ready",
      displayLabel: "guide.md",
      inlineContent: "# Guide\n",
      targetPath: "factory/docs/guide.md",
    });
  });
});
