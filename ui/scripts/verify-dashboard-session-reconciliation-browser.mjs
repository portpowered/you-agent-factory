import { chromium } from "playwright";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const storyID =
  "you-agent-factory-dashboard-session-tabs--canonical-list-reconciliation";
const viewports = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

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

    const navigation = page.getByRole("navigation", {
      name: "factory sessions",
    });
    await navigation.waitFor({ state: "visible" });

    if ((await navigation.getByRole("tab").count()) !== 2) {
      throw new Error(`Expected two canonical tabs at ${viewport.label}.`);
    }
    if ((await page.getByText("~default", { exact: true }).count()) !== 0) {
      throw new Error(
        `The default selector rendered as a tab at ${viewport.label}.`,
      );
    }

    await page.getByRole("button", { name: "Refresh sessions" }).click();
    await navigation.getByRole("tab", { name: "created" }).waitFor({
      state: "visible",
    });
    if (
      (await navigation.getByRole("tab", { name: "secondary" }).count()) !== 0
    ) {
      throw new Error(
        `The stale session remained after refresh at ${viewport.label}.`,
      );
    }
    if (
      (await navigation.getByRole("tab", { name: "removed" }).count()) !== 0
    ) {
      throw new Error(
        `The removed session rendered after refresh at ${viewport.label}.`,
      );
    }

    const tabs = navigation.getByRole("tab");
    await tabs.first().focus();
    await page.keyboard.press("End");
    await page.waitForFunction(
      (tab) => tab?.getAttribute("aria-selected") === "true",
      await tabs.nth(1).elementHandle(),
    );
    if ((await tabs.nth(1).getAttribute("aria-selected")) !== "true") {
      throw new Error(
        `End key did not select the last session at ${viewport.label}.`,
      );
    }
    await page.keyboard.press("Home");
    await page.waitForFunction(
      (tab) => tab?.getAttribute("aria-selected") === "true",
      await tabs.first().elementHandle(),
    );
    if ((await tabs.first().getAttribute("aria-selected")) !== "true") {
      throw new Error(
        `Home key did not restore the first session at ${viewport.label}.`,
      );
    }

    await page.getByRole("button", { name: "Fail refresh" }).click();
    await page
      .getByText("Factory sessions unavailable", { exact: true })
      .waitFor({
        state: "visible",
      });
    if (
      (await navigation.getByRole("tab", { name: "created" }).count()) !== 1
    ) {
      throw new Error(
        `Retained tabs disappeared after refresh failure at ${viewport.label}.`,
      );
    }
    await page.getByRole("button", { name: "Retry sessions" }).click();
    await page
      .getByText("Factory sessions unavailable", { exact: true })
      .waitFor({
        state: "hidden",
      });

    const metrics = await navigation.evaluate((element) => ({
      pageOverflows: document.documentElement.scrollWidth > window.innerWidth,
      tablistOverflows: element.scrollWidth > element.clientWidth,
    }));
    if (
      metrics.pageOverflows ||
      (viewport.width > 600 && metrics.tablistOverflows)
    ) {
      throw new Error(`Session list layout overflowed at ${viewport.label}.`);
    }

    console.log(
      `Verified canonical session reconciliation at ${viewport.label}.`,
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
  console.log("Dashboard session reconciliation browser verification passed.");
} finally {
  await browser.close();
}
