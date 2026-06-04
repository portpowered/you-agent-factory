import process from "node:process";
import { chromium } from "playwright";

import { storyUrl, waitForStoryRender } from "./storybook-responsive-helpers.mjs";

const STORYBOOK_HOST = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const STORYBOOK_PORT = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const STORYBOOK_URL = `http://${STORYBOOK_HOST}:${STORYBOOK_PORT}`;
const STORY_ID =
  "agent-factory-ui-color-role-overlay-hover-surfaces--overlay-hover-palette-verification";

const PALETTES = ["factory-dark", "factory-light"];

const OPAQUE_HOVER_TARGETS = [
  "hover-outline-button",
  "hover-secondary-button",
  "hover-list-row",
  "hover-panel-section",
  "hover-panel-compact",
];

const GHOST_HOVER_TARGET = "hover-ghost-button";

async function readBackground(page, testId) {
  return page.getByTestId(testId).evaluate((node) => getComputedStyle(node).backgroundColor);
}

async function readHoverBackground(page, testId) {
  const element = page.getByTestId(testId);
  await element.hover();
  return element.evaluate((node) => getComputedStyle(node).backgroundColor);
}

async function verifyPalette(page, paletteId) {
  await page.evaluate((palette) => {
    document.documentElement.dataset.colorPalette = palette;
  }, paletteId);

  const hoverBackgrounds = {};

  const ghostClassName = await page
    .getByTestId(GHOST_HOVER_TARGET)
    .evaluate((node) => node.className);
  if (!ghostClassName.includes("hover:bg-surface-container-low")) {
    throw new Error(
      `Expected ${GHOST_HOVER_TARGET} to keep hover:bg-surface-container-low on palette ${paletteId}.`,
    );
  }

  for (const testId of OPAQUE_HOVER_TARGETS) {
    const resting = await readBackground(page, testId);
    const hovered = await readHoverBackground(page, testId);
    if (resting === hovered) {
      throw new Error(
        `Expected ${testId} hover background to differ from resting on palette ${paletteId} (${resting}).`,
      );
    }
    hoverBackgrounds[testId] = hovered;
  }

  const selectedBackground = await readBackground(page, "selected-table-row");
  if (!selectedBackground || selectedBackground === "rgba(0, 0, 0, 0)") {
    throw new Error(
      `Expected selected-table-row to render a visible selected background on palette ${paletteId}.`,
    );
  }

  const tableRowClassName = await page
    .getByTestId("hover-table-row")
    .evaluate((node) => node.className);
  if (!tableRowClassName.includes("hover:bg-surface-container")) {
    throw new Error(
      `Expected hover-table-row to keep hover:bg-surface-container on palette ${paletteId}.`,
    );
  }

  return { hoverBackgrounds, selectedBackground };
}

export async function verifyOverlayHoverStorybook({
  storybookUrl = STORYBOOK_URL,
  storyId = STORY_ID,
} = {}) {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

  try {
    await page.goto(storyUrl(storybookUrl, storyId));
    await waitForStoryRender(page);

    const results = {};
    for (const paletteId of PALETTES) {
      results[paletteId] = await verifyPalette(page, paletteId);
    }

    for (const testId of OPAQUE_HOVER_TARGETS) {
      const dark = results["factory-dark"].hoverBackgrounds[testId];
      const light = results["factory-light"].hoverBackgrounds[testId];
      if (dark === light) {
        throw new Error(
          `Expected ${testId} hover background to differ between factory-dark (${dark}) and factory-light (${light}).`,
        );
      }
    }

    if (
      results["factory-dark"].selectedBackground ===
      results["factory-light"].selectedBackground
    ) {
      throw new Error(
        "Expected selected-table-row background to differ between factory-dark and factory-light.",
      );
    }
  } finally {
    await browser.close();
  }
}

async function main() {
  await verifyOverlayHoverStorybook();
  console.log(
    `Overlay hover palette verification passed for ${PALETTES.join(", ")} on ${STORY_ID}.`,
  );
}

if (import.meta.main) {
  await main();
}
