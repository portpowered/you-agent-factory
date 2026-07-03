import { describe, expect, test, vi } from "vitest";

import {
  PACKAGE_LAYOUT_DISPLAY_STORY_IDS,
  PACKAGE_LAYOUT_DISPLAY_VIEWPORTS,
  verifyPackageLayoutDisplayStories,
} from "./verify-package-layout-display-storybook-responsive.mjs";

describe("verify-package-layout-display-storybook-responsive", () => {
  test("exports representative typography, layout, and description-list story ids", () => {
    expect(PACKAGE_LAYOUT_DISPLAY_STORY_IDS.mobileTypographyRoles).toBe(
      "primitives-typography--mobile-typography-roles",
    );
    expect(PACKAGE_LAYOUT_DISPLAY_STORY_IDS.mobileSurfacePanelLayout).toBe(
      "layout-surfacepanel--mobile-surface-panel-layout",
    );
    expect(PACKAGE_LAYOUT_DISPLAY_STORY_IDS.desktopDescriptionList).toBe(
      "data-display-descriptionlist--desktop-description-list",
    );
    expect(PACKAGE_LAYOUT_DISPLAY_VIEWPORTS).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: "mobile", width: 390 }),
        expect.objectContaining({ label: "desktop", width: 1440 }),
      ]),
    );
  });

  test("verifyPackageLayoutDisplayStories exercises each responsive story check", async () => {
    const verifyStoryMock = vi.fn().mockResolvedValue(undefined);
    const browser = {
      close: vi.fn().mockResolvedValue(undefined),
      newPage: vi.fn().mockResolvedValue({}),
    };
    const chromium = {
      launch: vi.fn().mockResolvedValue(browser),
    };

    await verifyPackageLayoutDisplayStories({
      browserLauncher: chromium,
      storyChecks: [
        {
          label: "mobile typography roles",
          storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.mobileTypographyRoles,
          viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[0],
        },
        {
          label: "desktop action row layout",
          storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.desktopActionRowLayout,
          viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[1],
        },
      ],
      storybookUrl: "http://127.0.0.1:3817",
      verifyStory: verifyStoryMock,
    });

    expect(chromium.launch).toHaveBeenCalledTimes(1);
    expect(verifyStoryMock).toHaveBeenCalledTimes(2);
    expect(browser.close).toHaveBeenCalledTimes(1);
  });
});
