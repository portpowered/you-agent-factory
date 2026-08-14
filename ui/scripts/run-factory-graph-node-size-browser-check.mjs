import { chromium } from "playwright";
import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import {
  storyUrl,
  waitForStoryRegion,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const storybookUrl = `http://${host}:${port}`;
const storyId =
  "factory-graph-editor-visual-groups--editor-save-reload-workflow";
const workstationId = "workstation:intake";

const server = await ensureStorybookServer({ host, port: Number(port) });
const browser = await chromium.launch();

try {
  const page = await browser.newPage({
    viewport: { height: 900, width: 1440 },
  });
  page.setDefaultTimeout(60_000);

  await page.goto(storyUrl(storybookUrl, storyId), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);
  await page.evaluate(() => window.__resetVisualGroupEditorStory?.());
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);
  await waitForStoryRegion(page, "Work graph viewport");

  const readOnlyButton = await workstationButton(page);
  const initialDimensions = await nodeDimensions(readOnlyButton);
  if (await page.locator("[data-factory-graph-node-resize-actions]").count()) {
    throw new Error("Node resize actions rendered before entering edit mode.");
  }

  await page.getByRole("button", { name: "Edit mode" }).click();
  await page.getByRole("button", { name: "Leave editor" }).waitFor({
    state: "visible",
  });
  const selectedButton = await workstationButton(page);
  await selectedButton.click();
  await selectedButton.waitFor({ state: "visible" });
  await page.waitForFunction(
    (node) => node.getAttribute("aria-pressed") === "true",
    await selectedButton.elementHandle(),
  );

  const resizeActions = page.locator(
    "[data-factory-graph-node-resize-actions]",
  );
  await resizeActions.waitFor({ state: "visible" });
  const fitButton = page.getByRole("button", { name: "Fit to content" });
  const resetButton = page.getByRole("button", { name: "Reset size" });
  await fitButton.waitFor({ state: "visible" });
  await resetButton.waitFor({ state: "visible" });

  await fitButton.focus();
  await page.keyboard.press("Enter");
  await page.waitForFunction(
    () =>
      document.activeElement?.getAttribute("aria-label") === "Fit to content",
  );
  await resetButton.focus();
  await page.keyboard.press("Enter");
  await page.waitForFunction(
    () => document.activeElement?.getAttribute("aria-label") === "Reset size",
  );

  const resizeHandles = page.locator(".factory-graph-node-resize-control");
  if ((await resizeHandles.count()) !== 4) {
    throw new Error(
      `Expected four workstation resize handles, found ${await resizeHandles.count()}.`,
    );
  }
  const resizeHandle = resizeHandles.nth(3);
  await resizeHandle.waitFor({ state: "visible" });
  const handleBounds = await resizeHandle.boundingBox();
  if (!handleBounds) {
    throw new Error("Could not measure the workstation resize handle.");
  }

  const beforeResize = await nodeDimensions(selectedButton);
  await page.mouse.move(
    handleBounds.x + handleBounds.width / 2,
    handleBounds.y + handleBounds.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    handleBounds.x + handleBounds.width / 2 + 100,
    handleBounds.y + handleBounds.height / 2 + 60,
    { steps: 4 },
  );
  await page.mouse.up();

  await page.waitForFunction(
    ({ id, width, height }) => {
      const node = document.querySelector(`.react-flow__node[data-id="${id}"]`);
      return (
        node !== null &&
        Number.parseFloat(node.style.width) > width &&
        Number.parseFloat(node.style.height) > height
      );
    },
    {
      id: workstationId,
      width: beforeResize.width,
      height: beforeResize.height,
    },
  );

  const resizedDimensions = await nodeDimensions(selectedButton);
  if (
    resizedDimensions.width <= beforeResize.width ||
    resizedDimensions.height <= beforeResize.height
  ) {
    throw new Error(
      `Pointer resize did not increase both axes: before=${JSON.stringify(beforeResize)} after=${JSON.stringify(resizedDimensions)}.`,
    );
  }

  await page.getByRole("button", { name: "Save changes" }).click();
  await page
    .getByRole("heading", { name: "Save factory graph changes?" })
    .waitFor({ state: "visible" });
  await page.getByRole("button", { name: "Save layout" }).click();
  await page.getByText("Topology saved").waitFor({ state: "visible" });

  const savedSize = await page.evaluate((id) => {
    const factory = window.__getVisualGroupEditorPersistedFactory?.();
    return factory?.layout?.nodes?.find((node) => node.id === id)?.size ?? null;
  }, workstationId);
  if (
    !savedSize ||
    !Number.isFinite(savedSize.width) ||
    !Number.isFinite(savedSize.height) ||
    savedSize.width <= initialDimensions.width ||
    savedSize.height <= initialDimensions.height
  ) {
    throw new Error(
      `Saved Factory layout did not retain the resized dimensions: ${JSON.stringify(savedSize)}.`,
    );
  }

  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);
  const reloadedButton = await workstationButton(page);
  const reloadedDimensions = await nodeDimensions(reloadedButton);
  if (await page.locator("[data-factory-graph-node-resize-actions]").count()) {
    throw new Error(
      "Node resize actions rendered in read-only mode after reload.",
    );
  }
  assertDimensionsMatchSaved(reloadedDimensions, savedSize);

  await page.getByRole("button", { name: "Edit mode" }).click();
  await page.getByRole("button", { name: "Leave editor" }).waitFor({
    state: "visible",
  });
  const reselectedButton = await workstationButton(page);
  await reselectedButton.click();
  await page.waitForFunction(
    (node) => node.getAttribute("aria-pressed") === "true",
    await reselectedButton.elementHandle(),
  );
  await page
    .locator("[data-factory-graph-node-resize-actions]")
    .waitFor({ state: "visible" });
  assertDimensionsMatchSaved(await nodeDimensions(reselectedButton), savedSize);

  console.log(
    `Factory graph authored node-size browser verification passed: ${JSON.stringify(savedSize)} persisted through save/reload and read-only projection.`,
  );
} finally {
  await browser.close();
  await server.stop();
}

async function workstationButton(page) {
  const button = page.getByRole("button", {
    name: "Select Intake workstation",
  });
  await button.waitFor({ state: "visible" });
  return button;
}

async function nodeDimensions(button) {
  return button.evaluate((element) => {
    const node = element.closest(".react-flow__node");
    if (!node) {
      throw new Error(
        "Could not find the React Flow node for the workstation.",
      );
    }
    return {
      height: Number.parseFloat(
        node.style.height || window.getComputedStyle(node).height,
      ),
      width: Number.parseFloat(
        node.style.width || window.getComputedStyle(node).width,
      ),
    };
  });
}

function assertDimensionsMatchSaved(actual, saved) {
  if (
    Math.abs(actual.width - saved.width) > 1 ||
    Math.abs(actual.height - saved.height) > 1
  ) {
    throw new Error(
      `Reloaded Factory graph dimensions differed from authored size: saved=${JSON.stringify(saved)} actual=${JSON.stringify(actual)}.`,
    );
  }
}
