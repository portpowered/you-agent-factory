import { describe, expect, test, vi } from "vitest";

import {
  CHECKBOX_CONSISTENCY_VIEWPORTS,
  CURRENT_SELECTION_CHECKBOX_STORY_ID,
  FACTORY_GRAPH_EDITOR_CHECKBOX_STORY_ID,
  verifyCheckboxConsistencyStories,
} from "./verify-checkbox-consistency-storybook-responsive.mjs";

describe("verify-checkbox-consistency-storybook-responsive", () => {
  test("exports representative story ids and mobile/desktop viewports", () => {
    expect(CURRENT_SELECTION_CHECKBOX_STORY_ID).toContain(
      "checkbox-consistency-current-selection",
    );
    expect(FACTORY_GRAPH_EDITOR_CHECKBOX_STORY_ID).toContain(
      "checkbox-consistency-factory-graph-editor",
    );
    expect(CHECKBOX_CONSISTENCY_VIEWPORTS).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: "mobile", width: 390 }),
        expect.objectContaining({ label: "desktop", width: 1440 }),
      ]),
    );
  });

  test("verifyCheckboxConsistencyStories exercises both surfaces at each viewport", async () => {
    const verifyCurrentSelectionCheckboxSurfaceMock = vi
      .fn()
      .mockResolvedValue(undefined);
    const verifyFactoryGraphEditorCheckboxSurfaceMock = vi
      .fn()
      .mockResolvedValue(undefined);
    const verifySharedCheckboxStatesMock = vi.fn().mockResolvedValue(undefined);
    const browser = {
      close: vi.fn().mockResolvedValue(undefined),
      newPage: vi.fn().mockResolvedValue({}),
    };
    const chromium = {
      launch: vi.fn().mockResolvedValue(browser),
    };

    await verifyCheckboxConsistencyStories({
      browserLauncher: chromium,
      storybookUrl: "http://127.0.0.1:6008",
      verifyCurrentSelection: verifyCurrentSelectionCheckboxSurfaceMock,
      verifyFactoryGraphEditor: verifyFactoryGraphEditorCheckboxSurfaceMock,
      verifySharedStates: verifySharedCheckboxStatesMock,
      viewports: CHECKBOX_CONSISTENCY_VIEWPORTS,
    });

    expect(chromium.launch).toHaveBeenCalledTimes(1);
    expect(verifyCurrentSelectionCheckboxSurfaceMock).toHaveBeenCalledTimes(2);
    expect(verifyFactoryGraphEditorCheckboxSurfaceMock).toHaveBeenCalledTimes(
      2,
    );
    expect(verifySharedCheckboxStatesMock).toHaveBeenCalledTimes(1);
    expect(browser.close).toHaveBeenCalledTimes(1);
  });
});
