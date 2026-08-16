/**
 * Drives the real factory graph editor surface for visual group save/reload coverage.
 */

import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import {
  expectEditorGraphInteractions,
  expectGraphSurfaceBasics,
  expectRegionPointerThrough,
} from "./verify-factory-graph-visual-group-editor-interactions.mjs";

const TARGET_WORKSTATION_LABEL = "Plan";
const TARGET_WORKSTATION_ID = "workstation:plan";
const GROUP_LABEL = "Planning lane";
const CUSTOM_GROUP_COLOR = "#a1b2c3";

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

  const { toolbar, viewport } = await enterVisualGroupEditor(page);
  await createAndEditVisualGroup(page, viewport);
  await saveVisualGroup(page);

  await verifyVisualGroupAfterReload({
    currentUrl: page.url(),
    page,
    toolbar,
    waitForStoryRender,
  });
}

async function enterVisualGroupEditor(page) {
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

  return { toolbar, viewport };
}

async function createAndEditVisualGroup(page, viewport) {
  const targetWorkstationNode = page
    .locator(".react-flow__node")
    .filter({
      has: page.getByRole("button", {
        exact: true,
        name: `Select ${TARGET_WORKSTATION_LABEL} workstation`,
      }),
    })
    .first();
  await targetWorkstationNode.waitFor({ state: "visible" });
  await selectGraphNodeWithMarquee(page, viewport, targetWorkstationNode);

  await page.getByRole("button", { name: "Create group" }).click();
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "visible" });
  await expectVisualGroupControlsWithinViewport(page, viewport);

  const labelField = page.getByRole("textbox", { name: "Group label" });
  await labelField.fill(GROUP_LABEL);
  await expectLabelField(page, GROUP_LABEL);

  const membershipCheckbox = page.getByRole("checkbox", {
    name: `Include ${TARGET_WORKSTATION_LABEL} in this group`,
  });
  await expectChecked(membershipCheckbox, true);
  const secondaryMembershipCheckbox = page.getByRole("checkbox", {
    name: "Include Implement in this group",
  });
  const secondaryMembershipInputId =
    await secondaryMembershipCheckbox.getAttribute("id");
  if (!secondaryMembershipInputId) {
    throw new Error(
      "Expected the secondary membership Checkbox to have an id.",
    );
  }
  await page.locator(`label[for="${secondaryMembershipInputId}"]`).click();
  await expectChecked(secondaryMembershipCheckbox, true);
  await secondaryMembershipCheckbox.focus();
  await page.keyboard.press("Space");
  await expectChecked(secondaryMembershipCheckbox, false);
  await page.keyboard.press("Space");
  await expectChecked(secondaryMembershipCheckbox, true);

  const warningColorButton = page.getByRole("button", {
    exact: true,
    name: "Use warning group color",
  });
  await warningColorButton.click();
  await expectAttribute(warningColorButton, "aria-pressed", "true");

  const customColorPicker = page.getByLabel("Custom group color");
  await customColorPicker.fill(CUSTOM_GROUP_COLOR);
  await expectInputValue(customColorPicker, CUSTOM_GROUP_COLOR);
  await expectAttribute(warningColorButton, "aria-pressed", "false");

  const boundsBeforeResize = await readVisualGroupBounds(page);
  await resizeVisualGroup(page, "sw", { deltaX: -40, deltaY: 30 });
  const boundsAfterResize = await readVisualGroupBounds(page);
  expectBoundsChanged(boundsBeforeResize, boundsAfterResize, "resize");

  await page.getByRole("button", { name: "Fit to members" }).click();
  const boundsAfterFit = await readVisualGroupBounds(page);
  expectFiniteBounds(boundsAfterFit, "fit group");

  await dragVisualGroup(page, { deltaX: 12, deltaY: 8 });
  await resizeVisualGroup(page, "sw", { deltaX: -560, deltaY: 420 });
  await dragVisualGroup(page, { deltaX: 8, deltaY: 6 });

  const undoButton = page.getByRole("button", { name: "Undo" });
  await undoButton.waitFor({ state: "visible" });
  await expectEnabled(undoButton, true);
  await undoButton.click();
  const redoButton = page.getByRole("button", { name: "Redo" });
  await expectEnabled(redoButton, true);
  await redoButton.click();
}

async function saveVisualGroup(page) {
  await page.getByRole("button", { name: "Save changes" }).click();
  await page
    .getByRole("heading", { name: "Save factory graph changes?" })
    .waitFor({ state: "visible" });
  await page.getByRole("button", { name: "Save layout" }).click();
  await page.getByText("Topology saved").waitFor({ state: "visible" });

  const persistedFactory = await page.evaluate(() =>
    window.__getVisualGroupEditorPersistedFactory?.(),
  );
  const persistedGroup = persistedFactory?.layout?.groups?.find(
    (group) => group.label === GROUP_LABEL,
  );
  if (!persistedGroup) {
    throw new Error(`Saved factory did not contain the ${GROUP_LABEL} group.`);
  }
  if (persistedGroup.color !== CUSTOM_GROUP_COLOR) {
    throw new Error(
      `Expected the saved group color to be ${CUSTOM_GROUP_COLOR}, found ${persistedGroup.color}.`,
    );
  }
  if (!persistedGroup.nodeIds?.includes(TARGET_WORKSTATION_ID)) {
    throw new Error(
      `Expected the saved ${GROUP_LABEL} group to retain the selected ${TARGET_WORKSTATION_LABEL} member.`,
    );
  }
  expectFiniteBounds(persistedGroup.bounds, "saved group");
  await captureEvidence(page, "visual-group-before-reload");
}

async function verifyVisualGroupAfterReload({
  currentUrl,
  page,
  toolbar,
  waitForStoryRender,
}) {
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);
  if (page.url() !== currentUrl) {
    throw new Error(
      "Reload navigated away from the visual group editor story.",
    );
  }

  const restoredRegion = page.getByRole("region", {
    exact: true,
    name: GROUP_LABEL,
  });
  await restoredRegion.waitFor({ state: "visible" });
  await expectRegionPointerThrough(page, restoredRegion);
  const intakeObserverButton = page.getByRole("button", {
    exact: true,
    name: `Select ${TARGET_WORKSTATION_LABEL} workstation`,
  });
  await intakeObserverButton.click();
  await expectAttribute(intakeObserverButton, "aria-pressed", "true");
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "hidden" });
  await captureEvidence(page, "visual-group-after-reload-observer");

  const editModeAfterReload = page.getByRole("button", { name: "Edit mode" });
  await editModeAfterReload.waitFor({ state: "visible" });
  await editModeAfterReload.click();
  await toolbar.waitFor({ state: "visible" });
  await expectGraphSurfaceBasics(page);
  await page
    .getByRole("button", {
      exact: true,
      name: `Visual group ${GROUP_LABEL}`,
    })
    .waitFor({ state: "visible" });
  const reloadedGroup = page.getByRole("button", {
    exact: true,
    name: `Visual group ${GROUP_LABEL}`,
  });
  await reloadedGroup.focus();
  await page.keyboard.press("Enter");
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "visible" });
  await expectLabelField(page, GROUP_LABEL);
  const membershipCheckboxAfterReload = page.getByRole("checkbox", {
    name: `Include ${TARGET_WORKSTATION_LABEL} in this group`,
  });
  await expectChecked(membershipCheckboxAfterReload, true);
  await expectChecked(
    page.getByRole("checkbox", { name: "Include Implement in this group" }),
    true,
  );
  const customColorAfterReload = page.getByLabel("Custom group color");
  await expectInputValue(customColorAfterReload, CUSTOM_GROUP_COLOR);
  const warningColorAfterReload = page.getByRole("button", {
    exact: true,
    name: "Use warning group color",
  });
  await expectAttribute(warningColorAfterReload, "aria-pressed", "false");
  await expectEditorGraphInteractions(page);
  await captureEvidence(page, "visual-group-after-reload-editor");
}

async function captureEvidence(page, name) {
  const directory = process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;
  if (!directory) {
    return;
  }

  await mkdir(directory, { recursive: true });
  await page.screenshot({
    fullPage: true,
    path: join(directory, `${name}.png`),
  });
}

async function selectGraphNodeWithMarquee(page, viewport, locator) {
  const nodeBox = await locator.boundingBox();
  const viewportBox = await viewport.boundingBox();
  if (!nodeBox || !viewportBox) {
    throw new Error(
      "Could not measure the graph node and viewport for selection.",
    );
  }

  const startX = Math.max(viewportBox.x + 4, nodeBox.x - 20);
  const startY = Math.max(viewportBox.y + 4, nodeBox.y - 20);
  const endX = Math.min(
    viewportBox.x + viewportBox.width - 4,
    nodeBox.x + nodeBox.width + 20,
  );
  const endY = Math.min(
    viewportBox.y + viewportBox.height - 4,
    nodeBox.y + nodeBox.height + 20,
  );

  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(endX, endY, { steps: 5 });
  await page.mouse.up();
  await expectNodeSelected(page, locator);
}

async function expectNodeSelected(page, locator) {
  const handle = await locator.elementHandle();
  if (!handle) {
    throw new Error(
      `Could not resolve the ${TARGET_WORKSTATION_LABEL} graph node for selection.`,
    );
  }

  try {
    await page.waitForFunction(
      (node) => node.classList.contains("selected"),
      handle,
      { timeout: 5_000 },
    );
  } catch (error) {
    const details = await locator.evaluate((node) => ({
      ariaSelected: node.getAttribute("aria-selected"),
      className: node.className,
      html: node.outerHTML.slice(0, 800),
    }));
    throw new Error(
      `${TARGET_WORKSTATION_LABEL} node did not become selected: ${JSON.stringify(details)}`,
      {
        cause: error,
      },
    );
  }
}

async function expectAttribute(locator, name, expected) {
  const actual = await locator.getAttribute(name);
  if (actual !== expected) {
    throw new Error(
      `Expected ${name}=${expected} but found ${actual ?? "missing"}.`,
    );
  }
}

async function readVisualGroupBounds(page) {
  const group = page.locator("[data-factory-visual-group]").first();
  await group.waitFor({ state: "visible" });
  const bounds = await group.boundingBox();
  if (!bounds) {
    throw new Error("Could not measure the visual group bounds.");
  }

  return bounds;
}

async function expectVisualGroupControlsWithinViewport(page, viewport) {
  const controls = page.locator("[data-factory-visual-group-controls]");
  const controlsBox = await controls.boundingBox();
  const viewportBox = await viewport.boundingBox();
  const scrollMetrics = await controls.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));

  if (!controlsBox || !viewportBox) {
    throw new Error(
      "Could not measure the visual group controls and graph viewport.",
    );
  }

  const controlsBottom = controlsBox.y + controlsBox.height;
  const viewportBottom = viewportBox.y + viewportBox.height;
  if (
    controlsBox.y < viewportBox.y ||
    controlsBottom > viewportBottom ||
    controlsBox.x < viewportBox.x ||
    controlsBox.x + controlsBox.width > viewportBox.x + viewportBox.width
  ) {
    throw new Error(
      `Visual group controls exceed the graph viewport: ${JSON.stringify({
        controlsBox,
        viewportBox,
      })}`,
    );
  }

  if (scrollMetrics.scrollHeight <= scrollMetrics.clientHeight) {
    throw new Error(
      `Expected the visual group controls to expose vertical scrolling for the overflowing membership panel: ${JSON.stringify(
        scrollMetrics,
      )}`,
    );
  }
}

function expectBoundsChanged(before, after, operation) {
  const changed = ["height", "width", "x", "y"].some(
    (key) => Math.abs(before[key] - after[key]) > 1,
  );
  if (!changed) {
    throw new Error(
      `Expected visual group bounds to change after ${operation}: ${JSON.stringify(
        {
          after,
          before,
        },
      )}`,
    );
  }
}

function expectFiniteBounds(bounds, description) {
  if (
    !bounds ||
    ![bounds.height, bounds.width, bounds.x, bounds.y].every((value) =>
      Number.isFinite(value),
    ) ||
    bounds.height <= 0 ||
    bounds.width <= 0
  ) {
    throw new Error(
      `Expected ${description} bounds to be finite and positive.`,
    );
  }
}

async function dragVisualGroup(page, { deltaX, deltaY }) {
  const groupLabel = page.locator("[data-factory-visual-group-label]").first();
  await groupLabel.waitFor({ state: "visible" });
  const box = await groupLabel.boundingBox();
  if (!box) {
    throw new Error("Could not measure visual group label for drag.");
  }

  const startX = box.x + box.width / 2;
  const startY = box.y + box.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY);
  await page.mouse.up();
  await page.waitForTimeout(100);
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

  const boundsBeforePointerResize = await readVisualGroupBounds(page);
  const startX = box.x + box.width / 2;
  const startY = box.y + box.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY);
  await page.mouse.up();
  await page.waitForTimeout(100);

  const boundsAfterPointerResize = await readVisualGroupBounds(page);
  if (!areBoundsEqual(boundsBeforePointerResize, boundsAfterPointerResize)) {
    return;
  }

  await handle.focus();
  await pressResizeKey(handle, deltaX, "ArrowRight", "ArrowLeft");
  await pressResizeKey(handle, deltaY, "ArrowDown", "ArrowUp");
  await page.waitForTimeout(100);
}

async function pressResizeKey(locator, delta, positiveKey, negativeKey) {
  const key = delta >= 0 ? positiveKey : negativeKey;
  const presses = Math.max(1, Math.ceil(Math.abs(delta) / 16));
  for (let index = 0; index < presses; index += 1) {
    await locator.press(key);
  }
}

function areBoundsEqual(left, right) {
  return ["height", "width", "x", "y"].every(
    (key) => Math.abs(left[key] - right[key]) <= 1,
  );
}

async function expectLabelField(page, value) {
  const labelField = page.getByRole("textbox", { name: "Group label" });
  const actual = await labelField.inputValue();
  if (actual !== value) {
    throw new Error(`Expected group label "${value}" but found "${actual}".`);
  }
}

async function expectInputValue(locator, expected) {
  const actual = await locator.inputValue();
  if (actual !== expected) {
    throw new Error(
      `Expected input value "${expected}" but found "${actual}".`,
    );
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
