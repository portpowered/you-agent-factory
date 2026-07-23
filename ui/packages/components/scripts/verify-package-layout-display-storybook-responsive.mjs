import { chromium } from "playwright";

import {
  expectNoHorizontalOverflow,
  storyUrl,
  waitForStoryRender,
} from "../../../scripts/storybook-responsive-helpers.mjs";

export const PACKAGE_LAYOUT_DISPLAY_STORY_IDS = {
  mobileTypographyRoles: "primitives-typography--mobile-typography-roles",
  desktopTypographyRoles: "primitives-typography--desktop-typography-roles",
  mobileActionRowWrapping: "layout-actionrow--mobile-action-row-wrapping",
  desktopActionRowLayout: "layout-actionrow--desktop-action-row-layout",
  mobileSurfacePanelLayout: "layout-surfacepanel--mobile-surface-panel-layout",
  desktopSurfacePanelLayout:
    "layout-surfacepanel--desktop-surface-panel-layout",
  mobileDescriptionList:
    "data-display-descriptionlist--mobile-description-list",
  desktopDescriptionList:
    "data-display-descriptionlist--desktop-description-list",
};

export const PACKAGE_LAYOUT_DISPLAY_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

const STORY_CHECKS = [
  {
    label: "mobile typography roles",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.mobileTypographyRoles,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[0],
  },
  {
    label: "desktop typography roles",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.desktopTypographyRoles,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[1],
  },
  {
    label: "mobile action row wrapping",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.mobileActionRowWrapping,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[0],
  },
  {
    label: "desktop action row layout",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.desktopActionRowLayout,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[1],
  },
  {
    label: "mobile surface panel layout",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.mobileSurfacePanelLayout,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[0],
  },
  {
    label: "desktop surface panel layout",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.desktopSurfacePanelLayout,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[1],
  },
  {
    label: "mobile description list",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.mobileDescriptionList,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[0],
  },
  {
    label: "desktop description list",
    storyId: PACKAGE_LAYOUT_DISPLAY_STORY_IDS.desktopDescriptionList,
    viewport: PACKAGE_LAYOUT_DISPLAY_VIEWPORTS[1],
  },
];

export async function verifyPackageLayoutDisplayStory({
  page,
  storybookUrl,
  storyId,
  label,
  viewport,
}) {
  await page.setViewportSize({
    height: viewport.height,
    width: viewport.width,
  });
  await page.goto(storyUrl(storybookUrl, storyId), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);
  await expectNoHorizontalOverflow(page, `${label} (${viewport.label})`);
}

export async function verifyPackageLayoutDisplayStories({
  browserLauncher = chromium,
  storybookUrl,
  storyChecks = STORY_CHECKS,
  verifyStory = verifyPackageLayoutDisplayStory,
} = {}) {
  const browser = await browserLauncher.launch();
  const page = await browser.newPage();

  try {
    for (const storyCheck of storyChecks) {
      await verifyStory({
        label: storyCheck.label,
        page,
        storybookUrl,
        storyId: storyCheck.storyId,
        viewport: storyCheck.viewport,
      });
    }
  } finally {
    await browser.close();
  }
}
