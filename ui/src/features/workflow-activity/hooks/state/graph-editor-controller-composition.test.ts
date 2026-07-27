// Node-owned controller composition coverage.
import { describe, expect, it, vi } from "vitest";

import { composeGraphEditorControllers } from "./graph-editor-controller-composition";

describe("composeGraphEditorControllers", () => {
  it("combines the three controller boundaries without React", () => {
    const addEntityController = { reset: vi.fn() };
    const connectionController = {
      handleEditorConnect: vi.fn(),
      setConnectionNotice: vi.fn(),
    };
    const removalController = {
      handleEditorNodeDelete: vi.fn(),
      setPendingRemovalNodeId: vi.fn(),
    };

    expect(
      composeGraphEditorControllers(
        addEntityController,
        connectionController,
        removalController,
      ),
    ).toEqual({
      addEntityController,
      ...connectionController,
      ...removalController,
    });
  });
});
