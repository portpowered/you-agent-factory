import { chromium } from "playwright";

import {
  assertCheckboxCheckedState,
  assertCheckboxDisabledState,
  assertCheckboxInvalidState,
  assertStyledCheckboxTreatment,
  toggleCheckboxFromLabel,
  toggleCheckboxWithSpace,
} from "./checkbox-consistency-assertions.mjs";
import {
  expectNoHorizontalOverflow,
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

export const CURRENT_SELECTION_CHECKBOX_STORY_ID =
  "you-agent-factory-checkbox-consistency-current-selection--worker-skip-permissions";

export const CURRENT_SELECTION_INVALID_CHECKBOX_STORY_ID =
  "you-agent-factory-checkbox-consistency-current-selection--worker-skip-permissions-invalid";

export const FACTORY_GRAPH_EDITOR_CHECKBOX_STORY_ID =
  "you-agent-factory-checkbox-consistency-factory-graph-editor--cron-trigger-at-start";

export const SHARED_CHECKBOX_STORY_ID =
  "you-agent-factory-checkbox-consistency-shared-primitive--checkbox-state-showcase";

export const CHECKBOX_CONSISTENCY_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

const SKIP_PERMISSIONS_LABEL = "Bypass provider permissions";
const CRON_TRIGGER_LABEL = "Cron trigger at start";

export async function verifyCurrentSelectionCheckboxSurface({
  page,
  storybookUrl,
  viewport,
}) {
  await page.setViewportSize({
    height: viewport.height,
    width: viewport.width,
  });
  await page.goto(storyUrl(storybookUrl, CURRENT_SELECTION_CHECKBOX_STORY_ID), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);
  await expectNoHorizontalOverflow(
    page,
    `current-selection checkbox (${viewport.label})`,
  );

  const checkbox = page.getByRole("checkbox", {
    name: SKIP_PERMISSIONS_LABEL,
  });
  await assertStyledCheckboxTreatment(
    checkbox,
    `current-selection skipPermissions (${viewport.label})`,
  );
  await assertCheckboxCheckedState(
    checkbox,
    false,
    `current-selection skipPermissions initial (${viewport.label})`,
  );

  await toggleCheckboxFromLabel(page, SKIP_PERMISSIONS_LABEL);
  await assertCheckboxCheckedState(
    checkbox,
    true,
    `current-selection skipPermissions after label click (${viewport.label})`,
  );

  await toggleCheckboxWithSpace(page, checkbox);
  await assertCheckboxCheckedState(
    checkbox,
    false,
    `current-selection skipPermissions after Space (${viewport.label})`,
  );
}

export async function verifyFactoryGraphEditorCheckboxSurface({
  page,
  storybookUrl,
  viewport,
}) {
  await page.setViewportSize({
    height: viewport.height,
    width: viewport.width,
  });
  await page.goto(
    storyUrl(storybookUrl, FACTORY_GRAPH_EDITOR_CHECKBOX_STORY_ID),
    {
      timeout: 90_000,
      waitUntil: "networkidle",
    },
  );
  await waitForStoryRender(page);
  await expectNoHorizontalOverflow(
    page,
    `factory graph editor checkbox (${viewport.label})`,
  );

  const checkbox = page.getByRole("checkbox", {
    name: CRON_TRIGGER_LABEL,
  });
  await assertStyledCheckboxTreatment(
    checkbox,
    `factory graph editor cron trigger (${viewport.label})`,
  );
  await assertCheckboxCheckedState(
    checkbox,
    false,
    `factory graph editor cron trigger initial (${viewport.label})`,
  );

  await toggleCheckboxFromLabel(page, CRON_TRIGGER_LABEL);
  await assertCheckboxCheckedState(
    checkbox,
    true,
    `factory graph editor cron trigger after label click (${viewport.label})`,
  );

  await toggleCheckboxWithSpace(page, checkbox);
  await assertCheckboxCheckedState(
    checkbox,
    false,
    `factory graph editor cron trigger after Space (${viewport.label})`,
  );
}

export async function verifySharedCheckboxStates({ page, storybookUrl }) {
  await page.setViewportSize({ height: 900, width: 1440 });
  await page.goto(storyUrl(storybookUrl, SHARED_CHECKBOX_STORY_ID), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);

  const optionalCheckbox = page.getByRole("checkbox", {
    name: "Optional setting",
  });
  const disabledCheckbox = page.getByRole("checkbox", {
    name: "Disabled setting",
  });
  const invalidCheckbox = page.getByRole("checkbox", {
    name: "Invalid setting",
  });

  for (const [checkbox, label] of [
    [optionalCheckbox, "shared optional setting"],
    [disabledCheckbox, "shared disabled setting"],
    [invalidCheckbox, "shared invalid setting"],
  ]) {
    await assertStyledCheckboxTreatment(checkbox, label);
  }

  await assertCheckboxDisabledState(
    disabledCheckbox,
    "shared disabled setting",
  );
  await assertCheckboxInvalidState(invalidCheckbox, "shared invalid setting");

  await page.goto(
    storyUrl(storybookUrl, CURRENT_SELECTION_INVALID_CHECKBOX_STORY_ID),
    {
      timeout: 90_000,
      waitUntil: "networkidle",
    },
  );
  await waitForStoryRender(page);

  const invalidCurrentSelectionCheckbox = page.getByRole("checkbox", {
    name: SKIP_PERMISSIONS_LABEL,
  });
  await assertStyledCheckboxTreatment(
    invalidCurrentSelectionCheckbox,
    "current-selection invalid skipPermissions",
  );
  await assertCheckboxInvalidState(
    invalidCurrentSelectionCheckbox,
    "current-selection invalid skipPermissions",
  );
  await page
    .getByText("skipPermissions must be a boolean.")
    .waitFor({ state: "visible" });
}

export async function verifyCheckboxConsistencyStories({
  browserLauncher = chromium,
  storybookUrl,
  verifyCurrentSelection = verifyCurrentSelectionCheckboxSurface,
  verifyFactoryGraphEditor = verifyFactoryGraphEditorCheckboxSurface,
  verifySharedStates = verifySharedCheckboxStates,
  viewports = CHECKBOX_CONSISTENCY_VIEWPORTS,
} = {}) {
  const browser = await browserLauncher.launch();
  const page = await browser.newPage();

  try {
    for (const viewport of viewports) {
      await verifyCurrentSelection({
        page,
        storybookUrl,
        viewport,
      });
      await verifyFactoryGraphEditor({
        page,
        storybookUrl,
        viewport,
      });
    }

    await verifySharedStates({ page, storybookUrl });
  } finally {
    await browser.close();
  }
}
