import { chromium } from "playwright";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const stories = [
  "agent-factory-dashboard-work-chart--populated",
  "agent-factory-dashboard-work-chart--constrained-width",
];
const viewports = [
  { height: 812, width: 320 },
  { height: 900, width: 1280 },
];

async function verifyChart(page, storyID, viewport) {
  const chart = page.getByRole("img", { name: "Work outcome chart" });
  await chart.waitFor({ state: "visible" });

  const metrics = await chart.evaluate((element) => {
    const descriptionID = element.getAttribute("aria-describedby");
    const description = descriptionID
      ? document.getElementById(descriptionID)
      : null;
    const yAxisTitles = Array.from(
      element.querySelectorAll('[data-work-chart-overlay="true"]'),
    ).filter((overlay) => overlay.textContent?.includes("Work count"));

    return {
      description: description?.textContent ?? "",
      gridCount: element.querySelectorAll(".recharts-cartesian-grid").length,
      pageOverflows: document.documentElement.scrollWidth > window.innerWidth,
      svgCount: element.querySelectorAll("svg").length,
      width: element.getBoundingClientRect().width,
      yAxisTitleCount: yAxisTitles.length,
    };
  });

  if (metrics.gridCount !== 1 || metrics.svgCount !== 1) {
    throw new Error(
      `${storyID} rendered ${metrics.svgCount} SVGs and ${metrics.gridCount} grid treatments at ${viewport.width}px.`,
    );
  }
  if (metrics.yAxisTitleCount !== 1) {
    throw new Error(
      `${storyID} rendered ${metrics.yAxisTitleCount} Y-axis titles at ${viewport.width}px.`,
    );
  }
  if (!metrics.description.includes("Displayed values:")) {
    throw new Error(
      `${storyID} did not expose its visible values through aria-describedby at ${viewport.width}px.`,
    );
  }
  if (metrics.pageOverflows || metrics.width <= 0) {
    throw new Error(
      `${storyID} is not contained at ${viewport.width}px (overflow=${metrics.pageOverflows}, width=${metrics.width}).`,
    );
  }
}

const browser = await chromium.launch({ headless: true });
try {
  for (const storyID of stories) {
    const context = await browser.newContext({ viewport: viewports[0] });
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
      await verifyChart(page, storyID, viewports[0]);

      await page.setViewportSize(viewports[1]);
      await verifyChart(page, storyID, viewports[1]);
    } finally {
      await context.close();
    }
  }
  console.log("Work outcome chart browser verification passed.");
} finally {
  await browser.close();
}
