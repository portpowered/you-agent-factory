import { chromium } from "playwright";

function unique(values) {
  return [...new Set(values.filter((value) => value.length > 0))].sort();
}

function pathnameFor(urlString) {
  return new URL(urlString).pathname;
}

function assetPath(pathname) {
  return pathname.startsWith("/dashboard/ui/assets/");
}

function livePath(pathname) {
  return (
    pathname === "/events" || pathname === "/status" || pathname === "/work"
  );
}

async function waitForRenderedDashboard(page) {
  await page.getByRole("heading", { level: 1, name: "Infinite You" }).waitFor();
  await page.getByText("Work totals").waitFor();
  await page
    .getByRole("button", { name: "Select step-one workstation" })
    .waitFor();
  await page
    .getByRole("button", { name: "Select step-two workstation" })
    .waitFor();
  await page
    .getByRole("status", { name: "Infinite You event stream live" })
    .waitFor();
  await page.waitForFunction(() => {
    const workTotals = document.querySelector('[aria-label="work totals"]');
    if (!(workTotals instanceof HTMLElement)) {
      return false;
    }

    const articles = Array.from(workTotals.querySelectorAll("article"));
    return articles.some((article) => {
      const label = article.querySelector("span")?.textContent?.trim();
      const value = Number.parseInt(
        article.querySelector("strong")?.textContent?.trim() ?? "",
        10,
      );
      return label === "Completed" && Number.isFinite(value) && value > 0;
    });
  });
}

async function main() {
  const dashboardURL = process.argv[2];
  if (!dashboardURL) {
    throw new Error("usage: release-dashboard-smoke.mjs <dashboard-url>");
  }

  const assetRequests = [];
  const liveRequests = [];
  const pageErrors = [];
  const consoleErrors = [];
  const browser = await chromium.launch({ headless: true });

  try {
    const page = await browser.newPage();
    page.on("pageerror", (error) => {
      pageErrors.push(error.message);
    });
    page.on("console", (message) => {
      if (message.type() === "error") {
        consoleErrors.push(message.text());
      }
    });
    page.on("request", (request) => {
      const pathname = pathnameFor(request.url());
      if (assetPath(pathname)) {
        assetRequests.push(pathname);
      }
      if (livePath(pathname)) {
        liveRequests.push(pathname);
      }
    });

    const response = await page.goto(dashboardURL, {
      waitUntil: "domcontentloaded",
    });
    if (!response?.ok()) {
      throw new Error(
        `dashboard navigation failed with status ${response?.status() ?? "unknown"}`,
      );
    }

    await waitForRenderedDashboard(page);

    if (pageErrors.length > 0) {
      throw new Error(`dashboard page errors: ${pageErrors.join(" | ")}`);
    }
    if (consoleErrors.length > 0) {
      throw new Error(`dashboard console errors: ${consoleErrors.join(" | ")}`);
    }

    const observedAssetPaths = unique(assetRequests);
    const observedLivePaths = unique(liveRequests);
    if (observedAssetPaths.length === 0) {
      throw new Error(
        "dashboard did not request any embedded /dashboard/ui/assets resources",
      );
    }
    if (!observedLivePaths.includes("/events")) {
      throw new Error("dashboard did not establish a live /events request");
    }

    const streamStatusName = await page
      .getByRole("status", { name: "Infinite You event stream live" })
      .getAttribute("aria-label");
    if (streamStatusName !== "Infinite You event stream live") {
      throw new Error(
        `dashboard stream status name = ${JSON.stringify(streamStatusName)}, want "Infinite You event stream live"`,
      );
    }

    const visibleTexts = unique([
      "Infinite You",
      "Work totals",
      "step-one",
      "step-two",
    ]);
    process.stdout.write(
      `${JSON.stringify(
        {
          assetRequestPaths: observedAssetPaths,
          liveRequestPaths: observedLivePaths,
          streamStatusName,
          visibleTexts,
        },
        null,
        2,
      )}\n`,
    );
  } finally {
    await browser.close();
  }
}

await main();
