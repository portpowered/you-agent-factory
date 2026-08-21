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
const workstationId = "workstation:plan";
const browserCheckTimeoutMs = 60_000;

const server = await ensureStorybookServer({ host, port: Number(port) });
const browser = await chromium.launch();

try {
  const page = await browser.newPage({
    viewport: { height: 900, width: 1440 },
  });
  page.setDefaultTimeout(browserCheckTimeoutMs);

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
  if (await page.locator(".factory-graph-node-resize-grip").count()) {
    throw new Error("Node resize controls rendered before entering edit mode.");
  }

  await page.getByRole("button", { name: "Edit mode" }).click();
  await page.getByRole("button", { name: "Leave editor" }).waitFor({
    state: "visible",
  });
  const selectedButton = await selectWorkstation(page);
  await waitForAttachedEdges(page, workstationId);

  // One grip per node, not one per graph: every editable workstation carries
  // its own, so scope the count to the node under test.
  const resizeControl = page.locator(
    `.react-flow__node[data-id="${workstationId}"] .factory-graph-node-resize-grip`,
  );
  await resizeControl.waitFor({ state: "visible" });
  if ((await resizeControl.count()) !== 1) {
    throw new Error(
      `Expected one node resize grip, found ${await resizeControl.count()}.`,
    );
  }
  if (
    (await page.getByRole("button", { name: "Fit to content" }).count()) > 0 ||
    (await page.getByRole("button", { name: "Reset size" }).count()) > 0
  ) {
    throw new Error(
      "Obsolete Fit to content or Reset size node actions rendered in edit mode.",
    );
  }

  const attachedEdgeIdsBeforeResize = await attachedEdgeIds(page);
  if (attachedEdgeIdsBeforeResize.length === 0) {
    throw new Error("The selected workstation has no attached graph edges.");
  }
  const workstationHandleIds = await page
    .locator(`[data-nodeid="${workstationId}"]`)
    .evaluateAll((handles) =>
      handles
        .map((handle) => handle.getAttribute("data-handleid"))
        .filter((handleId) => handleId !== null),
    );
  if (
    !workstationHandleIds.includes("workstation-input-target") ||
    !workstationHandleIds.includes("workstation-output-source")
  ) {
    throw new Error(
      `The selected workstation is missing attached input/output handles: ${JSON.stringify(workstationHandleIds)}.`,
    );
  }

  const handleBounds = await resizeControl.boundingBox();
  if (!handleBounds) {
    throw new Error("Could not measure the workstation resize grip.");
  }

  const beforeResize = await nodeDimensions(selectedButton);
  if (
    (await selectedButton.locator(
      "[data-workstation-runtime-label], [data-workstation-scheduling-label]",
    ).count()) !== 0
  ) {
    throw new Error(
      "Collapsed workstation rendered expanded runtime or scheduling content.",
    );
  }
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
  const expandedDetails = selectedButton.locator(
    "[data-workstation-runtime-label], [data-workstation-scheduling-label]",
  );
  if ((await expandedDetails.count()) === 0) {
    throw new Error(
      "Committed workstation resize did not render expanded runtime or scheduling content.",
    );
  }
  await expandedDetails.first().waitFor({ state: "visible" });
  const attachedEdgeIdsAfterResize = await attachedEdgeIds(page);
  if (
    JSON.stringify(attachedEdgeIdsAfterResize) !==
    JSON.stringify(attachedEdgeIdsBeforeResize)
  ) {
    throw new Error(
      `Attached graph edges changed during resize: before=${JSON.stringify(attachedEdgeIdsBeforeResize)} after=${JSON.stringify(attachedEdgeIdsAfterResize)}.`,
    );
  }

  const undoButton = page.getByRole("button", { name: "Undo" });
  await undoButton.waitFor({ state: "visible" });
  await undoButton.click();
  await page.waitForFunction(
    ({ id, width, height }) => {
      const node = document.querySelector(`.react-flow__node[data-id="${id}"]`);
      if (!node) {
        return false;
      }
      const actualWidth = Number.parseFloat(node.style.width);
      const actualHeight = Number.parseFloat(node.style.height);
      return (
        Math.abs(actualWidth - width) <= 1 &&
        Math.abs(actualHeight - height) <= 1
      );
    },
    {
      id: workstationId,
      width: beforeResize.width,
      height: beforeResize.height,
    },
  );
  assertDimensionsMatchSaved(
    await nodeDimensions(selectedButton),
    beforeResize,
  );

  const redoButton = page.getByRole("button", { name: "Redo" });
  await redoButton.click();
  await page.waitForFunction(
    ({ id, width, height }) => {
      const node = document.querySelector(`.react-flow__node[data-id="${id}"]`);
      if (!node) {
        return false;
      }
      const actualWidth = Number.parseFloat(node.style.width);
      const actualHeight = Number.parseFloat(node.style.height);
      return (
        Math.abs(actualWidth - width) <= 1 &&
        Math.abs(actualHeight - height) <= 1
      );
    },
    {
      id: workstationId,
      width: resizedDimensions.width,
      height: resizedDimensions.height,
    },
  );
  assertDimensionsMatchSaved(
    await nodeDimensions(selectedButton),
    resizedDimensions,
  );

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
  if (await page.locator(".factory-graph-node-resize-grip").count()) {
    throw new Error(
      "Node resize controls rendered in read-only mode after reload.",
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
    .locator(
      `.react-flow__node[data-id="${workstationId}"] .factory-graph-node-resize-grip`,
    )
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
  const node = page.locator(`.react-flow__node[data-id="${workstationId}"]`);
  await node.waitFor({
    state: "visible",
    timeout: browserCheckTimeoutMs,
  });
  const button = node.getByRole("button", {
    name: "Select Plan workstation",
  });
  await button.waitFor({
    state: "visible",
    timeout: browserCheckTimeoutMs,
  });
  return button;
}

async function selectWorkstation(page) {
  const button = await workstationButton(page);
  if ((await button.getAttribute("aria-pressed")) !== "true") {
    await button.click();
  }
  await page.waitForFunction(
    (id) =>
      document
        .querySelector(`.react-flow__node[data-id="${id}"]`)
        ?.querySelector('button[aria-label="Select Plan workstation"]')
        ?.getAttribute("aria-pressed") === "true",
    workstationId,
    { timeout: browserCheckTimeoutMs },
  );
  return workstationButton(page);
}

async function nodeDimensions(button) {
  return button.evaluate((element) => {
    const node = element.closest(".react-flow__node");
    if (!node) {
      throw new Error(
        "Could not find the React Flow node for the Plan workstation.",
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

async function attachedEdgeIds(page) {
  return page
    .locator(".react-flow__edge")
    .evaluateAll(
      (edges, nodeId) =>
        edges
          .map((edge) => edge.getAttribute("data-id"))
          .filter((edgeId) => edgeId?.includes(nodeId)),
      workstationId,
    );
}

async function waitForAttachedEdges(page, nodeId) {
  await page.waitForFunction(
    (id) => {
      return Array.from(document.querySelectorAll(".react-flow__edge")).some(
        (edge) => edge.getAttribute("data-id")?.includes(id),
      );
    },
    nodeId,
    { timeout: browserCheckTimeoutMs },
  );
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
