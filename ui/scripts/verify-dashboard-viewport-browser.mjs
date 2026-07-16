import { chromium } from "playwright";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const stories = [
  "you-agent-factory-workflow-dashboard--dashboard-improvements-smoke",
  "you-agent-factory-workflow-dashboard--dashboard-responsive-empty",
  "you-agent-factory-workflow-dashboard--dashboard-responsive-error",
  "you-agent-factory-workflow-dashboard--dashboard-responsive-loading",
];

const viewports = [
  { height: 812, width: 320 },
  { height: 812, width: 375 },
  { height: 900, width: 768 },
  { height: 900, width: 1280 },
];

async function verifyViewport(browser, storyID, viewport) {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);
  const storyURL = new URL(
    `/iframe.html?id=${storyID}&viewMode=story`,
    storybookURL,
  ).toString();

  try {
    await page.goto(storyURL, {
      timeout: 30_000,
      waitUntil: "domcontentloaded",
    });
    const shell = page.getByRole("main");
    await shell.waitFor({ state: "visible" });
    await page
      .getByRole("region", { name: "you-agent-factory bento board" })
      .waitFor({ state: "visible" });

    const metrics = await shell.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return {
        leftGutter:
          rect.left + Number.parseFloat(getComputedStyle(element).paddingLeft),
        rightGutter:
          window.innerWidth -
          rect.right +
          Number.parseFloat(getComputedStyle(element).paddingRight),
        pageOverflows: document.documentElement.scrollWidth > window.innerWidth,
      };
    });
    if (metrics.pageOverflows) {
      throw new Error(`Dashboard widened the page at ${viewport.width}px.`);
    }
    if (
      viewport.width < 640 &&
      (metrics.leftGutter > 4 || metrics.rightGutter > 4)
    ) {
      throw new Error(
        `Dashboard exceeded the 4px phone gutter at ${viewport.width}px.`,
      );
    }
  } finally {
    await context.close();
  }
}

const browser = await chromium.launch({ headless: true });
try {
  for (const storyID of stories) {
    for (const viewport of viewports) {
      await verifyViewport(browser, storyID, viewport);
    }
  }
  console.log("Dashboard viewport browser verification passed.");
} finally {
  await browser.close();
}
