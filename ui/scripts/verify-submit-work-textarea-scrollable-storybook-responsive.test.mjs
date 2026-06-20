import { describe, expect, test, vi } from "vitest";

import {
  SUBMIT_WORK_LONG_TEXT_SCROLLABLE_STORY_ID,
  SUBMIT_WORK_TEXTAREA_SCROLLABLE_VIEWPORTS,
  verifySubmitWorkTextareaScrollableStories,
} from "./verify-submit-work-textarea-scrollable-storybook-responsive.mjs";

describe("verify-submit-work-textarea-scrollable-storybook-responsive", () => {
  test("exports the long-text story id and mobile/tablet/desktop viewports", () => {
    expect(SUBMIT_WORK_LONG_TEXT_SCROLLABLE_STORY_ID).toContain(
      "submit-work-card--long-text-scrollable-verification",
    );
    expect(SUBMIT_WORK_TEXTAREA_SCROLLABLE_VIEWPORTS).toEqual([
      { height: 844, label: "mobile", width: 390 },
      { height: 1024, label: "tablet", width: 768 },
      { height: 900, label: "desktop", width: 1440 },
    ]);
  });

  test("verifySubmitWorkTextareaScrollableStories exercises each viewport", async () => {
    const verifySurfaceMock = vi.fn().mockResolvedValue(undefined);
    const browser = {
      close: vi.fn().mockResolvedValue(undefined),
      newPage: vi.fn().mockResolvedValue({}),
    };
    const chromium = {
      launch: vi.fn().mockResolvedValue(browser),
    };

    await verifySubmitWorkTextareaScrollableStories({
      browserLauncher: chromium,
      storybookUrl: "http://127.0.0.1:6008",
      verifySurface: verifySurfaceMock,
      viewports: SUBMIT_WORK_TEXTAREA_SCROLLABLE_VIEWPORTS,
    });

    expect(chromium.launch).toHaveBeenCalledTimes(1);
    expect(verifySurfaceMock).toHaveBeenCalledTimes(3);
    expect(browser.close).toHaveBeenCalledTimes(1);
  });
});
