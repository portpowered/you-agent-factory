import { chromium } from "playwright";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const storyURL = new URL(
  "/iframe.html?id=you-agent-factory-dashboard-dashboard-header--responsive-verification&viewMode=story",
  storybookURL,
).toString();

const viewports = [
  { height: 812, width: 320 },
  { height: 812, width: 375 },
  { height: 900, width: 768 },
  { height: 900, width: 1280 },
];

async function verifyViewport(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);

  try {
    await page.goto(storyURL, {
      timeout: 30_000,
      waitUntil: "commit",
    });
    const header = page.locator("[data-dashboard-panel-shell='panel']");
    await header.waitFor({ state: "visible" });
    await page.getByRole("tablist").waitFor({ state: "visible" });
    const headerClassName = await header.getAttribute("class");
    if (!headerClassName?.includes("sticky")) {
      throw new Error(
        `Header was not fixed at the dashboard top at ${viewport.width}px.`,
      );
    }
    if (
      (await page.getByRole("slider", { name: "Timeline tick" }).count()) > 0
    ) {
      throw new Error(
        `Timeline controls remained in the fixed header at ${viewport.width}px.`,
      );
    }

    const hasPageOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    if (hasPageOverflow) {
      throw new Error(`Header widened the page at ${viewport.width}px.`);
    }

    const buttonCount = await header.getByRole("button").count();
    for (let index = 0; index < buttonCount; index += 1) {
      const button = header.getByRole("button").nth(index);
      const bounds = await button.boundingBox();
      if (!bounds || bounds.width < 40 || bounds.height < 40) {
        throw new Error(
          `Header action ${index + 1} did not retain a 40px touch target at ${viewport.width}px.`,
        );
      }
    }

    const navigation = page.getByRole("navigation", {
      name: "Factory sessions",
    });
    const navigationMetrics = await navigation.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    if (navigationMetrics.scrollWidth > navigationMetrics.clientWidth) {
      throw new Error(
        `Session tabs overflowed instead of sharing the available width at ${viewport.width}px.`,
      );
    }
    if ((await navigation.evaluate((element) => element.scrollLeft)) !== 0) {
      throw new Error(
        `Session tabs unexpectedly retained a horizontal scroll position at ${viewport.width}px.`,
      );
    }

    const regions = await page.evaluate(() => {
      const top = document.querySelector("[data-dashboard-header-top-region]");
      const tabs = document.querySelector("[data-dashboard-header-tab-region]");
      if (!top || !tabs) {
        throw new Error("Expected fixed header regions were not rendered.");
      }
      return {
        tabsTop: tabs.getBoundingClientRect().top,
        topBottom: top.getBoundingClientRect().bottom,
      };
    });
    if (viewport.width <= 375 && !(regions.tabsTop >= regions.topBottom)) {
      throw new Error(
        `Header tab strip was not stacked below the top actions at ${viewport.width}px.`,
      );
    }
  } finally {
    await context.close();
  }
}

const browser = await chromium.launch({ headless: true });
try {
  for (const viewport of viewports) {
    await verifyViewport(browser, viewport);
  }
  console.log("Dashboard header responsive browser verification passed.");
} finally {
  await browser.close();
}
