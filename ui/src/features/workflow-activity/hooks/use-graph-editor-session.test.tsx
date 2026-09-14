import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { baseFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import { useGraphEditorSession } from "./use-graph-editor-session";

describe("useGraphEditorSession", () => {
  it("does not enable editing when the successful state has no document", () => {
    const { result } = renderHook(() =>
      useGraphEditorSession({
        activeTool: null,
        editorMode: true,
        locale: "en",
        onAttemptLeaveEditor: vi.fn(),
        onLeaveEditor: vi.fn(),
        sessionState: {
          currentFactoryDefinition: null,
          definitionStatus: "success",
          hasPendingGraphChanges: false,
          isSaving: false,
          projectedFactory: null,
        },
        setActiveTool: vi.fn(),
        setEditorMode: vi.fn(),
      }),
    );

    expect(result.current.canInteractWithEditor).toBe(false);
  });

  it("enables editor controls for a successful authoritative document", () => {
    const { result } = renderHook(() =>
      useGraphEditorSession({
        activeTool: null,
        editorMode: true,
        locale: "en",
        onAttemptLeaveEditor: vi.fn(),
        onLeaveEditor: vi.fn(),
        sessionState: {
          currentFactoryDefinition: baseFactoryDefinition,
          definitionStatus: "success",
          hasPendingGraphChanges: false,
          isSaving: false,
          projectedFactory: baseFactoryDefinition,
        },
        setActiveTool: vi.fn(),
        setEditorMode: vi.fn(),
      }),
    );

    expect(result.current.canInteractWithEditor).toBe(true);
    expect(result.current.addMenuActions.length).toBeGreaterThan(0);
  });
});
