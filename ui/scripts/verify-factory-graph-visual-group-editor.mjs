/**
 * Drives the real factory graph editor surface for visual group save/reload coverage.
 */

export async function verifyFactoryGraphVisualGroupEditorWorkflow({
  page,
  storyUrl,
  waitForStoryRender,
}) {
  await page.goto(storyUrl, {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);
  await page.evaluate(() => {
    window.__resetVisualGroupEditorStory?.();
  });
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);

  const viewport = page.getByRole("region", { name: "Work graph viewport" });
  await viewport.waitFor({ state: "visible" });

  const editModeButton = page.getByRole("button", { name: "Edit mode" });
  await editModeButton.waitFor({ state: "visible" });
  await editModeButton.click();
  await page.getByRole("button", { name: "Leave editor" }).waitFor({
    state: "visible",
  });

  const toolbar = page.getByRole("region", {
    name: "Factory graph editor tools",
  });
  await toolbar.waitFor({ state: "visible" });

  await page.getByRole("button", { name: "Create group" }).click();
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "visible" });

  const labelField = page.getByRole("textbox", { name: "Group label" });
  await labelField.fill("Planning lane");
  await expectLabelField(page, "Planning lane");

  const membershipCheckbox = page.getByRole("checkbox", {
    name: "Include Intake in this group",
  });
  await membershipCheckbox.check();
  await expectChecked(membershipCheckbox, true);

  await dragVisualGroup(page, { deltaX: 24, deltaY: 16 });
  await resizeVisualGroup(page, "se", { deltaX: 40, deltaY: 30 });

  await page.getByRole("button", { name: "Delete group" }).click();
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "hidden" });

  await page.getByRole("button", { name: "Create group" }).click();
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "visible" });
  await labelField.fill("Planning lane");
  await membershipCheckbox.check();

  await dragVisualGroup(page, { deltaX: 12, deltaY: 8 });
  await resizeVisualGroup(page, "se", { deltaX: 24, deltaY: 20 });
  await dragVisualGroup(page, { deltaX: 8, deltaY: 6 });

  const undoButton = page.getByRole("button", { name: "Undo" });
  await undoButton.waitFor({ state: "visible" });
  await expectEnabled(undoButton, true);
  await undoButton.click();
  const redoButton = page.getByRole("button", { name: "Redo" });
  await expectEnabled(redoButton, true);
  await redoButton.click();

  await page.getByRole("button", { name: "Save changes" }).click();
  await page
    .getByRole("heading", { name: "Save factory graph changes?" })
    .waitFor({ state: "visible" });
  await page.getByRole("button", { name: "Save layout" }).click();
  await page.getByText("Topology saved").waitFor({ state: "visible" });

  const currentUrl = page.url();
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);
  if (page.url() !== currentUrl) {
    throw new Error(
      "Reload navigated away from the visual group editor story.",
    );
  }

  const editModeAfterReload = page.getByRole("button", { name: "Edit mode" });
  await editModeAfterReload.waitFor({ state: "visible" });
  await editModeAfterReload.click();
  await toolbar.waitFor({ state: "visible" });
  await page
    .getByRole("button", { name: "Visual group Planning lane" })
    .waitFor({ state: "visible" });
  const reloadedGroup = page.getByRole("button", {
    name: "Visual group Planning lane",
  });
  await reloadedGroup.focus();
  await page.keyboard.press("Enter");
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "visible" });
  await expectLabelField(page, "Planning lane");
  const membershipCheckboxAfterReload = page.getByRole("checkbox", {
    name: "Include Intake in this group",
  });
  await expectChecked(membershipCheckboxAfterReload, true);
}

async function dragVisualGroup(page, { deltaX, deltaY }) {
  const groupBody = page.locator("[data-factory-visual-group-body]").first();
  await groupBody.waitFor({ state: "visible" });
  const box = await groupBody.boundingBox();
  if (!box) {
    throw new Error("Could not measure visual group body for drag.");
  }

  const startX = box.x + box.width / 2;
  const startY = box.y + box.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY);
  await page.mouse.up();
}

async function resizeVisualGroup(page, corner, { deltaX, deltaY }) {
  const handle = page
    .locator(`[data-factory-visual-group-resize="${corner}"]`)
    .first();
  await handle.waitFor({ state: "visible" });
  const box = await handle.boundingBox();
  if (!box) {
    throw new Error(`Could not measure visual group resize handle ${corner}.`);
  }

  const startX = box.x + box.width / 2;
  const startY = box.y + box.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY);
  await page.mouse.up();
}

async function expectLabelField(page, value) {
  const labelField = page.getByRole("textbox", { name: "Group label" });
  const actual = await labelField.inputValue();
  if (actual !== value) {
    throw new Error(`Expected group label "${value}" but found "${actual}".`);
  }
}

async function expectChecked(locator, checked) {
  const actual = await locator.isChecked();
  if (actual !== checked) {
    throw new Error(
      `Expected checkbox checked=${checked} but found ${actual}.`,
    );
  }
}

async function expectEnabled(locator, enabled) {
  const actual = await locator.isEnabled();
  if (actual !== enabled) {
    throw new Error(`Expected control enabled=${enabled} but found ${actual}.`);
  }
}
