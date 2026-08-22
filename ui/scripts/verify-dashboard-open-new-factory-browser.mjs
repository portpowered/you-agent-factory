import { chromium } from "playwright";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const storyID =
  "you-agent-factory-dashboard-session-tabs-open-new-factory--journey";
const viewports = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

async function clickFixtureControl(page, label) {
  const button = page
    .locator("[data-factory-journey-controls] button")
    .filter({ hasText: label })
    .first();
  await button.waitFor({ state: "attached" });
  await button.evaluate((element) => {
    element.click();
  });
}

async function waitForAttribute(locator, attribute, expectedValue) {
  for (let attempt = 0; attempt < 300; attempt += 1) {
    if ((await locator.getAttribute(attribute)) === expectedValue) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(
    `Expected ${attribute} to become ${expectedValue} on the selected tab.`,
  );
}

async function waitForFocus(locator, label) {
  for (let attempt = 0; attempt < 300; attempt += 1) {
    if (
      await locator.evaluate((element) => document.activeElement === element)
    ) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`${label} did not regain focus after the dialog closed.`);
}

async function resolveInitialSessionList(page) {
  await clickFixtureControl(page, "Resolve initial session list");
  await page.getByRole("tab", { name: "dashboard-regression" }).waitFor({
    state: "visible",
  });
}

async function expectNoPageOverflow(page, label) {
  const metrics = await page.evaluate(() => ({
    pageOverflows: document.documentElement.scrollWidth > window.innerWidth,
  }));
  if (metrics.pageOverflows) {
    throw new Error(`Open/New Factory widened the page at ${label}.`);
  }
}

async function openFactoryValidationFailure(page) {
  const openFactoryButton = page.getByRole("button", { name: "Open Factory" });
  await openFactoryButton.click();
  const dialog = page.getByRole("dialog", { name: "Open Factory" });
  await dialog
    .getByRole("textbox", { name: "Factory folder" })
    .fill("/workspace/dashboard-regression-missing");
  await dialog.getByRole("button", { name: "Start Factory" }).click();
  await dialog.getByRole("button", { name: "Checking folder..." }).waitFor({
    state: "visible",
  });
  await clickFixtureControl(page, "Resolve Open validation failure");
  await page
    .getByRole("alert")
    .filter({ hasText: "could not verify this factory folder" })
    .waitFor({ state: "visible" });
  if ((await page.getByRole("status").count()) !== 0) {
    throw new Error("Open validation failure showed a success status.");
  }
  await dialog.getByRole("button", { name: "Close dialog" }).click();
  await dialog.waitFor({ state: "hidden" });
  await waitForFocus(openFactoryButton, "Open Factory");
}

async function openFactorySuccessfully(page) {
  await page.getByRole("button", { name: "Open Factory" }).click();
  const dialog = page.getByRole("dialog", { name: "Open Factory" });
  await dialog
    .getByRole("textbox", { name: "Factory folder" })
    .fill("/workspace/dashboard-regression");
  await dialog.getByRole("button", { name: "Start Factory" }).click();
  await dialog.getByRole("button", { name: "Checking folder..." }).waitFor({
    state: "visible",
  });
  await clickFixtureControl(page, "Resolve folder validation");

  const targetPicker = page.getByRole("region", {
    name: "Pick a runnable target",
  });
  await targetPicker.waitFor({ state: "visible" });
  await targetPicker.getByRole("button", { name: /secondary/ }).click();
  if ((await page.getByRole("status").count()) !== 0) {
    throw new Error(
      "Selecting an Open Factory target showed success before the final action.",
    );
  }

  await targetPicker
    .getByRole("button", { name: "Open selected target" })
    .click();
  await targetPicker
    .getByRole("button", { name: "Opening target..." })
    .waitFor({
      state: "visible",
    });
  await clickFixtureControl(page, "Resolve Open success");
  const secondaryTab = page.getByRole("tab", { name: "secondary" });
  await secondaryTab.waitFor({
    state: "visible",
  });
  await waitForAttribute(secondaryTab, "aria-selected", "true");
  await page
    .getByRole("status")
    .filter({ hasText: "Factory opened successfully." })
    .waitFor({ state: "visible" });
  await dialog.waitFor({ state: "hidden" });
}

async function cancelNewFactoryPreview(page) {
  // The New Factory journey now starts from the Open Factory dialog: an
  // uninitialized folder validates into a scaffold preview with its own
  // Create Factory confirmation.
  const openFactoryButton = page.getByRole("button", { name: "Open Factory" });
  await openFactoryButton.click();
  const dialog = page.getByRole("dialog", { name: "Open Factory" });
  await dialog
    .getByRole("textbox", { name: "Factory folder" })
    .fill("/workspace/dashboard-regression-new");
  await dialog.getByRole("button", { name: "Start Factory" }).click();
  await dialog.getByRole("button", { name: "Checking folder..." }).waitFor({
    state: "visible",
  });
  await clickFixtureControl(page, "Resolve New validation");
  await page
    .locator("[data-factory-preview-target]")
    .filter({ hasText: "/workspace/dashboard-regression-new/factory" })
    .waitFor({ state: "visible" });
  if ((await page.locator('[role="tab"]').count()) !== 2) {
    throw new Error(
      "New Factory validation changed session membership before confirmation.",
    );
  }

  await dialog.getByRole("button", { name: "Cancel" }).click();
  await page
    .locator("[data-factory-preview-target]")
    .waitFor({ state: "detached" });
  await dialog
    .getByRole("textbox", { name: "Factory folder" })
    .waitFor({ state: "visible" });
  if ((await page.locator('[role="tab"]').count()) !== 2) {
    throw new Error("Cancelling New Factory changed session membership.");
  }
  await dialog.getByRole("button", { name: "Close dialog" }).click();
  await dialog.waitFor({ state: "hidden" });
  await waitForFocus(openFactoryButton, "Open Factory");
}

async function createNewFactorySuccessfully(page) {
  await page.getByRole("button", { name: "Open Factory" }).click();
  const dialog = page.getByRole("dialog", { name: "Open Factory" });
  await dialog
    .getByRole("textbox", { name: "Factory folder" })
    .fill("/workspace/dashboard-regression-new");
  await dialog.getByRole("button", { name: "Start Factory" }).click();
  await dialog.getByRole("button", { name: "Checking folder..." }).waitFor({
    state: "visible",
  });
  await clickFixtureControl(page, "Resolve New validation");
  await page
    .locator("[data-factory-preview-target]")
    .filter({ hasText: "/workspace/dashboard-regression-new/factory" })
    .waitFor({ state: "visible" });
  await dialog.getByRole("button", { name: "Create Factory" }).click();
  await dialog.getByRole("button", { name: "Creating Factory..." }).waitFor({
    state: "visible",
  });
  await clickFixtureControl(page, "Resolve New success");
  const createdTab = page.getByRole("tab", { name: "created" });
  await createdTab.waitFor({ state: "visible" });
  await waitForAttribute(createdTab, "aria-selected", "true");
  await page
    .getByRole("status")
    .filter({ hasText: "New Factory created and opened successfully." })
    .waitFor({ state: "visible" });
  if ((await page.getByText("~default", { exact: true }).count()) !== 0) {
    throw new Error("The default selector rendered during New Factory flow.");
  }
  await dialog.waitFor({ state: "hidden" });
}

async function failNewFactoryCreation(page) {
  const openFactoryButton = page.getByRole("button", { name: "Open Factory" });
  await openFactoryButton.click();
  const dialog = page.getByRole("dialog", { name: "Open Factory" });
  await dialog
    .getByRole("textbox", { name: "Factory folder" })
    .fill("/workspace/dashboard-regression-new-broken");
  await dialog.getByRole("button", { name: "Start Factory" }).click();
  await dialog.getByRole("button", { name: "Checking folder..." }).waitFor({
    state: "visible",
  });
  await clickFixtureControl(page, "Resolve New broken validation");
  await page
    .locator("[data-factory-preview-target]")
    .filter({ hasText: "/workspace/dashboard-regression-new-broken/factory" })
    .waitFor({ state: "visible" });
  await dialog.getByRole("button", { name: "Create Factory" }).click();
  await dialog.getByRole("button", { name: "Creating Factory..." }).waitFor({
    state: "visible",
  });
  await clickFixtureControl(page, "Resolve New failure");
  await page
    .getByRole("alert")
    .filter({ hasText: "The new Factory could not be created." })
    .waitFor({ state: "visible" });
  if ((await page.getByRole("status").count()) !== 0) {
    throw new Error("New Factory failure showed a success status.");
  }
  if ((await page.locator('[role="tab"]').count()) !== 2) {
    throw new Error("New Factory failure changed session membership.");
  }
  await dialog.getByRole("button", { name: "Close dialog" }).click();
  await dialog.waitFor({ state: "hidden" });
  await waitForFocus(openFactoryButton, "Open Factory");
}

async function verifyViewport(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);

  try {
    await page.goto(
      new URL(
        `/iframe.html?id=${storyID}&viewMode=story`,
        storybookURL,
      ).toString(),
      { timeout: 30_000, waitUntil: "domcontentloaded" },
    );

    await resolveInitialSessionList(page);
    if ((await page.getByRole("tab").count()) !== 2) {
      throw new Error(`Expected two initial sessions at ${viewport.label}.`);
    }
    if ((await page.getByText("~default", { exact: true }).count()) !== 0) {
      throw new Error(`The default selector rendered at ${viewport.label}.`);
    }

    await openFactoryValidationFailure(page);
    await openFactorySuccessfully(page);
    await cancelNewFactoryPreview(page);
    await failNewFactoryCreation(page);
    await createNewFactorySuccessfully(page);
    await expectNoPageOverflow(page, viewport.label);

    console.log(
      `Verified Open/New Factory journeys at ${viewport.label} (${viewport.width}px).`,
    );
  } finally {
    await context.close();
  }
}

const browser = await chromium.launch({ headless: true });
try {
  for (const viewport of viewports) {
    await verifyViewport(browser, viewport);
  }
  console.log("Dashboard Open/New Factory browser verification passed.");
} finally {
  await browser.close();
}
