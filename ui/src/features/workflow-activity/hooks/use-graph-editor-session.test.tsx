import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

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
});
