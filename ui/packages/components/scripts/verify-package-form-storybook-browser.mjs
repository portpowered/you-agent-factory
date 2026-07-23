import {
  expectCheckboxFocusRingVisible,
  expectNoHorizontalOverflow,
  expectTextLikeFocusRingVisible,
  expectVisibleLabelWithinViewport,
  waitForStoryRender,
} from "./verify-package-storybook-browser-helpers.mjs";

export const PACKAGE_INPUT_MOBILE_STORY_ID = "forms-packageinput--mobile-width";
export const PACKAGE_TEXTAREA_MOBILE_STORY_ID =
  "forms-packagetextarea--mobile-width";
export const PACKAGE_CHECKBOX_MOBILE_STORY_ID =
  "forms-packagecheckbox--mobile-width";
export const PACKAGE_FILE_INPUT_MOBILE_STORY_ID =
  "forms-packagefileinput--mobile-width";

export const PACKAGE_INPUT_FOCUS_STORY_ID = "forms-packageinput--focus";
export const PACKAGE_TEXTAREA_FOCUS_STORY_ID = "forms-packagetextarea--focus";
export const PACKAGE_CHECKBOX_FOCUS_STORY_ID = "forms-packagecheckbox--focus";
export const PACKAGE_FILE_INPUT_FOCUS_STORY_ID =
  "forms-packagefileinput--focus";

export const PACKAGE_FORM_MOBILE_STORY_IDS = [
  PACKAGE_INPUT_MOBILE_STORY_ID,
  PACKAGE_TEXTAREA_MOBILE_STORY_ID,
  PACKAGE_CHECKBOX_MOBILE_STORY_ID,
  PACKAGE_FILE_INPUT_MOBILE_STORY_ID,
];

export const PACKAGE_FORM_FOCUS_STORY_IDS = [
  PACKAGE_INPUT_FOCUS_STORY_ID,
  PACKAGE_TEXTAREA_FOCUS_STORY_ID,
  PACKAGE_CHECKBOX_FOCUS_STORY_ID,
  PACKAGE_FILE_INPUT_FOCUS_STORY_ID,
];

export const PACKAGE_FORM_RESPONSIVE_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

function formStoryLabel(storyId) {
  if (storyId.includes("textarea")) {
    return "Factory notes";
  }
  if (storyId.includes("checkbox")) {
    return "Enable cron trigger";
  }
  if (storyId.includes("fileinput")) {
    return "Factory cover image";
  }
  return "Factory name";
}

export async function verifyPackageFormMobileStories({
  page,
  storyIds = PACKAGE_FORM_MOBILE_STORY_IDS,
  storyUrl,
  viewports = PACKAGE_FORM_RESPONSIVE_VIEWPORTS,
} = {}) {
  for (const viewport of viewports) {
    await page.setViewportSize({
      height: viewport.height,
      width: viewport.width,
    });

    for (const storyId of storyIds) {
      await page.goto(storyUrl(storyId), {
        timeout: 90_000,
        waitUntil: "networkidle",
      });
      await waitForStoryRender(page);
      await expectNoHorizontalOverflow(page, `${storyId} (${viewport.label})`);
      await expectVisibleLabelWithinViewport(
        page,
        formStoryLabel(storyId),
        viewport,
      );
    }
  }
}

export async function verifyPackageFormFocusStories({
  page,
  storyIds = PACKAGE_FORM_FOCUS_STORY_IDS,
  storyUrl,
} = {}) {
  await page.setViewportSize({ height: 900, width: 1440 });

  for (const storyId of storyIds) {
    await page.goto(storyUrl(storyId), {
      timeout: 90_000,
      waitUntil: "networkidle",
    });
    await waitForStoryRender(page);

    if (storyId.includes("checkbox")) {
      await expectCheckboxFocusRingVisible(
        page,
        "Enable cron trigger",
        storyId,
      );
      continue;
    }

    const selector = storyId.includes("fileinput")
      ? 'input[type="file"]'
      : storyId.includes("textarea")
        ? "textarea"
        : 'input[type="text"]';
    await expectTextLikeFocusRingVisible(page, selector, storyId);
  }
}
