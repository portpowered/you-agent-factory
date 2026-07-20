import assert from "node:assert/strict";

import { chromium } from "playwright";

import {
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const storybookUrl =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const interactiveStory = "agent-factory-emulator-adapter-demo--interactive";
const failureStory = "agent-factory-emulator-adapter-demo--submission-failure";
const viewports = [
  { height: 844, width: 390 },
  { height: 900, width: 1440 },
];

async function openStory(page, storyID) {
  await page.goto(storyUrl(storybookUrl, storyID), {
    timeout: 30_000,
    waitUntil: "domcontentloaded",
  });
  await waitForStoryRender(page);
  try {
    await page.getByText("Tick 2 of 2").waitFor({ timeout: 10_000 });
  } catch (error) {
    const body = (await page.locator("body").innerText()).slice(0, 1_200);
    throw new Error(
      `${storyID} did not initialize its retained events. ${body}`,
      {
        cause: error,
      },
    );
  }
}

async function assertViewportAndLayout(page, viewport, storyID) {
  const actualViewport = await page.evaluate(() => ({
    height: window.innerHeight,
    width: window.innerWidth,
  }));
  assert.deepEqual(
    actualViewport,
    viewport,
    `${storyID} did not run at the required browser viewport.`,
  );
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  );
  assert.equal(
    overflow,
    false,
    `${storyID} has page-level horizontal overflow at ${viewport.width}x${viewport.height}.`,
  );

  for (const control of await page.getByRole("button").all()) {
    const box = await control.boundingBox();
    if (!box) continue;
    assert.ok(
      box.x >= -1 && box.x + box.width <= viewport.width + 1,
      `A ${storyID} control is outside the ${viewport.width}px viewport.`,
    );
  }
}

async function pressControl(page, name) {
  const control = page.getByRole("button", { name });
  await control.focus();
  assert.equal(
    await control.evaluate((element) => element === document.activeElement),
    true,
    `${name} did not receive keyboard focus.`,
  );
  await page.keyboard.press("Enter");
}

async function verifyInteractiveFlow(page, viewport) {
  await openStory(page, interactiveStory);
  await assertViewportAndLayout(page, viewport, interactiveStory);

  const slider = page.getByRole("slider", { name: "Select replay tick" });
  await slider.focus();
  await page.keyboard.press("ArrowLeft");
  await page.getByText("Tick 1 of 2").waitFor();
  await page.getByText("Viewing Factory history.").waitFor();

  const textbox = page.getByRole("textbox", { name: "Submit text" });
  assert.equal(
    await textbox.isDisabled(),
    true,
    "History inspection did not disable emulator submission.",
  );

  await pressControl(page, "Accept next event");
  await page.getByText("Tick 1 of 3").waitFor();
  assert.equal(
    await slider.inputValue(),
    "1",
    "A new accepted event moved the historical selection.",
  );

  await pressControl(page, "Follow latest");
  await page.getByText("Tick 3 of 3").waitFor();
  assert.equal(
    await textbox.isEnabled(),
    true,
    "Following the head did not restore current-mode submission.",
  );

  await pressControl(page, "Play");
  const runtimeStatus = page.getByRole("status", { name: "Runtime status" });
  await runtimeStatus.getByText("Playing").waitFor();
  assert.equal(await runtimeStatus.getAttribute("data-playing"), "true");
  await pressControl(page, "Pause");
  await runtimeStatus.getByText("Paused").waitFor();
  assert.equal(await runtimeStatus.getAttribute("data-playing"), "false");

  await textbox.fill("Browser task");
  await textbox.press("Enter");
  await page.waitForFunction(
    (element) => element instanceof HTMLTextAreaElement && element.value === "",
    await textbox.elementHandle(),
  );

  await textbox.fill("First line");
  await textbox.press("Shift+Enter");
  await textbox.type("Second line");
  assert.equal(
    await textbox.inputValue(),
    "First line\nSecond line",
    "Shift+Enter did not preserve a multiline draft.",
  );
  await assertViewportAndLayout(page, viewport, interactiveStory);
}

async function verifyFailureFlow(page, viewport) {
  await openStory(page, failureStory);
  await assertViewportAndLayout(page, viewport, failureStory);
  const textbox = page.getByRole("textbox", { name: "Submit text" });
  await textbox.fill("Keep this draft");
  await textbox.press("Enter");
  await page
    .getByRole("alert")
    .getByText("The emulator could not accept this work.")
    .waitFor();
  assert.equal(
    await textbox.inputValue(),
    "Keep this draft",
    "A failed submission cleared the controlled draft.",
  );
  await page
    .getByRole("status", { name: "Runtime status" })
    .getByText("Error")
    .waitFor();
  assert.equal(
    await page.getByRole("button", { name: "Play" }).isDisabled(),
    true,
  );
  assert.equal(
    await page.getByRole("button", { name: "Step" }).isDisabled(),
    true,
  );
  await assertViewportAndLayout(page, viewport, failureStory);
}

async function verifyFactoryEmulatorAdapterBrowser() {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const viewport of viewports) {
      const context = await browser.newContext({ viewport });
      const page = await context.newPage();
      await verifyInteractiveFlow(page, viewport);
      await verifyFailureFlow(page, viewport);
      await context.close();
    }
  } finally {
    await browser.close();
  }
}

await verifyFactoryEmulatorAdapterBrowser();
console.log("Factory emulator adapter browser checks passed.");
