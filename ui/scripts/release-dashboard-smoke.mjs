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
    pathname === "/events" ||
    pathname === "/status" ||
    pathname === "/work" ||
    /^\/factory-sessions\/[^/]+\/(events|status|work)$/.test(pathname)
  );
}

const RETIRED_BRAND_PATTERNS = [/finite you/i, /Infinite You/i];

async function waitForRenderedDashboard(page) {
  await page.locator('[aria-label="work totals"]').waitFor();
  await page
    .getByRole("button", { name: "Select step-one workstation" })
    .waitFor();
  await page
    .getByRole("button", { name: "Select step-two workstation" })
    .waitFor();
  await page.waitForFunction(() => {
    const workTotals = document.querySelector('[aria-label="work totals"]');
    if (!(workTotals instanceof HTMLElement)) {
      return false;
    }

    const articles = Array.from(workTotals.querySelectorAll("article"));
    return articles.some((article) => {
      const label = article
        .querySelector("span")
        ?.textContent?.trim()
        .toLowerCase();
      const value = Number.parseInt(
        article.querySelector("strong")?.textContent?.trim() ?? "",
        10,
      );
      const accessibleLabel =
        article.getAttribute("aria-label")?.trim().toLowerCase() ?? "";
      return (
        (label === "completed" || accessibleLabel.startsWith("completed:")) &&
        Number.isFinite(value) &&
        value > 0
      );
    });
  });
}

async function readMetadata(page) {
  return page.evaluate(() => ({
    pageTitle: document.title,
    metaDescription:
      document
        .querySelector('meta[name="description"]')
        ?.getAttribute("content")
        ?.trim() ?? "",
  }));
}

async function readVisibleTexts(page) {
  return page.evaluate(() => {
    const selectors = [
      '[role="heading"][aria-level="1"]',
      'article[aria-label="Work totals"]',
      '[aria-label="work totals"]',
      'button[aria-label="Select step-one workstation"]',
      'button[aria-label="Select step-two workstation"]',
    ];
    return selectors
      .map(
        (selector) =>
          document.querySelector(selector)?.textContent?.trim() ?? "",
      )
      .filter((value) => value.length > 0);
  });
}

function ensureNoRetiredBranding(values, contextLabel) {
  for (const value of values) {
    for (const pattern of RETIRED_BRAND_PATTERNS) {
      if (pattern.test(value)) {
        throw new Error(
          `${contextLabel} contained retired branding: ${JSON.stringify(value)}`,
        );
      }
    }
  }
}

async function closeBrowser(browser) {
  await Promise.race([
    browser.close(),
    new Promise((resolve) => {
      setTimeout(resolve, 2000);
    }),
  ]);
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
    const { pageTitle, metaDescription } = await readMetadata(page);
    const visibleTexts = unique(await readVisibleTexts(page));
    if (pageTitle !== "You Agent Factory Dashboard") {
      throw new Error(
        `dashboard page title = ${JSON.stringify(pageTitle)}, want "You Agent Factory Dashboard"`,
      );
    }
    if (
      metaDescription !==
      "Standalone live dashboard shell for You Agent Factory."
    ) {
      throw new Error(
        `dashboard meta description = ${JSON.stringify(metaDescription)}, want "Standalone live dashboard shell for You Agent Factory."`,
      );
    }
    ensureNoRetiredBranding(
      [pageTitle, metaDescription, ...visibleTexts],
      "dashboard smoke evidence",
    );

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
    const eventStreamPath = observedLivePaths.find(
      (pathname) => pathname === "/events" || pathname.endsWith("/events"),
    );
    if (!eventStreamPath) {
      throw new Error(
        "dashboard did not establish a live event stream request",
      );
    }

    process.stdout.write(
      `${JSON.stringify(
        {
          assetRequestPaths: observedAssetPaths,
          liveRequestPaths: observedLivePaths,
          pageTitle,
          metaDescription,
          streamStatusName: eventStreamPath,
          visibleTexts,
        },
        null,
        2,
      )}\n`,
    );
  } finally {
    await closeBrowser(browser);
  }
}

try {
  await main();
  process.exit(0);
} catch (error) {
  console.error(error);
  process.exit(1);
}
