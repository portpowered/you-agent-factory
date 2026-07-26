// Node-owned editor-mode decision coverage.
import { describe, expect, it } from "vitest";

import { resolveGraphEditorModeToggleAction } from "./graph-editor-session-state";

describe("resolveGraphEditorModeToggleAction", () => {
  it.each([
    [false, false, undefined, "enter"],
    [true, true, undefined, "confirm-leave"],
    [true, false, undefined, "leave"],
    [false, false, "Classify", "blocked"],
  ] as const)(
    "resolves editorMode=%s changed=%s unavailable=%s to %s",
    (editorMode, hasPendingGraphChanges, unavailableClassifierWorkstationName, expected) => {
      expect(
        resolveGraphEditorModeToggleAction({
          editorMode,
          hasPendingGraphChanges,
          unavailableClassifierWorkstationName,
        }),
      ).toBe(expected);
    },
  );
});
