import { chromium } from "playwright";

import {
  assertSubmitWorkTextareaOverflows,
  assertSubmitWorkTextareaScrollTreatment,
  verifyKeyboardTextareaNavigation,
} from "./submit-work-textarea-scrollable-assertions.mjs";
import {
  expectNoHorizontalOverflow,
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

export const SUBMIT_WORK_LONG_TEXT_SCROLLABLE_STORY_ID =
  "agent-factory-dashboard-submit-work-card--long-text-scrollable-verification";

export const SUBMIT_WORK_TEXTAREA_SCROLLABLE_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 1024, label: "tablet", width: 768 },
  { height: 900, label: "desktop", width: 1440 },
];

export async function verifySubmitWorkLongTextScrollableSurface({
  page,
  storybookUrl,
  viewport,
}) {
  await page.setViewportSize({
    height: viewport.height,
    width: viewport.width,
  });
  await page.goto(
    storyUrl(storybookUrl, SUBMIT_WORK_LONG_TEXT_SCROLLABLE_STORY_ID),
    {
      timeout: 90_000,
      waitUntil: "networkidle",
    },
  );
  await waitForStoryRender(page);
  await expectNoHorizontalOverflow(
    page,
    `submit-work long text textarea (${viewport.label})`,
  );

  const card = page.getByRole("article", { name: "Submit work" });
  await card.waitFor({ state: "visible" });

  const submissionTextarea = card.getByRole("textbox", { name: "Text item 1" });
  await assertSubmitWorkTextareaScrollTreatment(
    submissionTextarea,
    `submit-work submission textarea (${viewport.label})`,
  );
  await assertSubmitWorkTextareaOverflows(
    submissionTextarea,
    `submit-work submission textarea (${viewport.label})`,
  );

  await card.getByRole("button", { name: "Submit work" }).waitFor({
    state: "visible",
  });

  await verifyKeyboardTextareaNavigation(page, card, viewport);
}

export async function verifySubmitWorkTextareaScrollableStories({
  browserLauncher = chromium,
  storybookUrl,
  verifySurface = verifySubmitWorkLongTextScrollableSurface,
  viewports = SUBMIT_WORK_TEXTAREA_SCROLLABLE_VIEWPORTS,
} = {}) {
  const browser = await browserLauncher.launch();
  const page = await browser.newPage();

  try {
    for (const viewport of viewports) {
      await verifySurface({
        page,
        storybookUrl,
        viewport,
      });
    }
  } finally {
    await browser.close();
  }
}
