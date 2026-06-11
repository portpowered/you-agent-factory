import { chromium } from "playwright";
import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import {
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const storybookUrl = `http://${host}:${port}`;
const storyId =
  "factory-graph-editor-visual-groups--save-reload-workflow";

const server = await ensureStorybookServer({ host, port: Number(port) });

try {
  const browser = await chromium.launch();
  const page = await browser.newPage({
    viewport: { height: 900, width: 1440 },
  });
  await page.goto(storyUrl(storybookUrl, storyId), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);

  const workflow = page.locator("[data-visual-group-workflow-phase]").first();
  await workflow.waitFor({ state: "visible" });

  const click = async (name) => {
    await page.getByRole("button", { name }).click();
  };

  await click("Create group");
  await expectPhase(page, "created");
  await click("Rename group");
  await expectPhase(page, "renamed");
  await expectLabel(page, "Planning lane");
  await click("Assign node");
  await expectPhase(page, "assigned");
  await expectMemberCount(page, "1");
  await click("Move group");
  await expectPhase(page, "moved");
  await click("Resize group");
  await expectPhase(page, "resized");
  await click("Delete group");
  await expectPhase(page, "deleted");
  await click("Create group");
  await click("Rename group");
  await click("Assign node");
  await click("Move group");
  await click("Resize group");
  await click("Save layout");
  await expectPhase(page, "saved");
  await expectLayoutDirty(page, "false");
  await click("Reload layout");
  await expectPhase(page, "reloaded");
  await expectLabel(page, "Planning lane");
  await click("Move group");
  await click("Undo layout");
  await expectPhase(page, "undone");
  await click("Redo layout");
  await expectPhase(page, "redone");

  console.log("Visual group save/reload browser verification passed.");
  await browser.close();
} finally {
  await server.stop();
}

async function expectPhase(page, phase) {
  await page
    .locator("[data-visual-group-workflow-phase]")
    .first()
    .waitFor({ state: "attached" });
  const actual = await page
    .locator("[data-visual-group-workflow-phase]")
    .first()
    .getAttribute("data-visual-group-workflow-phase");
  if (actual !== phase) {
    throw new Error(`Expected workflow phase "${phase}" but found "${actual}".`);
  }
}

async function expectLabel(page, label) {
  const actual = await page
    .locator("[data-visual-group-selected-label]")
    .first()
    .textContent();
  if (actual?.trim() !== label) {
    throw new Error(`Expected group label "${label}" but found "${actual}".`);
  }
}

async function expectMemberCount(page, count) {
  const actual = await page
    .locator("[data-visual-group-member-count]")
    .first()
    .textContent();
  if (actual?.trim() !== count) {
    throw new Error(`Expected member count "${count}" but found "${actual}".`);
  }
}

async function expectLayoutDirty(page, value) {
  const actual = await page
    .locator("[data-visual-group-layout-dirty]")
    .first()
    .textContent();
  if (actual?.trim() !== value) {
    throw new Error(`Expected layout dirty "${value}" but found "${actual}".`);
  }
}
