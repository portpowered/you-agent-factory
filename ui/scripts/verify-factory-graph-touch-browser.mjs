import { chromium } from "playwright";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const storyURL = new URL(
  "/iframe.html?id=agent-factory-dashboard-react-flow-current-activity-card--touch-pane-panning&viewMode=story",
  storybookURL,
).toString();

async function openStory(browser, contextOptions) {
  const context = await browser.newContext(contextOptions);
  const page = await context.newPage();
  page.setDefaultTimeout(60_000);
  await page.goto(storyURL, {
    timeout: 60_000,
    waitUntil: "domcontentloaded",
  });
  await page.locator(".react-flow__pane").waitFor({ state: "visible" });
  await page.locator(".react-flow__viewport").waitFor({ state: "visible" });
  return { context, page };
}

async function emptyPanePoint(page) {
  const point = await page.locator(".react-flow__pane").evaluate((pane) => {
    const bounds = pane.getBoundingClientRect();
    const candidates = [];
    for (let y = bounds.top + 80; y < bounds.bottom - 20; y += 40) {
      for (let x = bounds.left + 20; x < bounds.right - 20; x += 40) {
        if (document.elementFromPoint(x, y) === pane) {
          candidates.push({ x, y });
        }
      }
    }
    const centerX = bounds.left + bounds.width / 2;
    const centerY = bounds.top + bounds.height / 2;
    return (
      candidates.sort(
        (left, right) =>
          Math.hypot(left.x - centerX, left.y - centerY) -
          Math.hypot(right.x - centerX, right.y - centerY),
      )[0] ?? null
    );
  });

  if (!point) {
    throw new Error("Could not find an unoccupied graph pane point");
  }
  return point;
}

async function viewportTransform(page) {
  return page
    .locator(".react-flow__viewport")
    .evaluate((viewport) => viewport.style.transform);
}

async function waitForAnimationFrame(page) {
  await page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => resolve())),
  );
}

async function dispatchTouchMove(client, page, touchPoints) {
  await client.send("Input.dispatchTouchEvent", {
    touchPoints,
    type: "touchMove",
  });
  await waitForAnimationFrame(page);
}

async function verifyTouchGestures(browser) {
  const { context, page } = await openStory(browser, {
    hasTouch: true,
    isMobile: true,
    viewport: { height: 812, width: 375 },
  });

  try {
    const client = await context.newCDPSession(page);
    const nodeButtons = page.locator(".react-flow__node button");
    let station = null;
    let stationBounds = null;
    for (let index = 0; index < (await nodeButtons.count()); index += 1) {
      const candidate = nodeButtons.nth(index);
      const candidateBounds = await candidate.boundingBox();
      if (!candidateBounds) {
        continue;
      }
      const isTopmost = await candidate.evaluate((button) => {
        const bounds = button.getBoundingClientRect();
        const target = document.elementFromPoint(
          bounds.x + bounds.width / 2,
          bounds.y + bounds.height / 2,
        );
        return target === button || target?.closest("button") === button;
      });
      if (isTopmost) {
        station = candidate;
        stationBounds = candidateBounds;
        break;
      }
    }
    if (!station || !stationBounds) {
      throw new Error("No graph node was reachable for a touch tap");
    }
    await page.touchscreen.tap(
      stationBounds.x + stationBounds.width / 2,
      stationBounds.y + stationBounds.height / 2,
    );
    await page.waitForFunction(
      (button) => button.getAttribute("aria-pressed") === "true",
      await station.elementHandle(),
    );

    const point = await emptyPanePoint(page);
    const initialTransform = await viewportTransform(page);
    const initialScroll = await page.evaluate(() => ({
      x: document.documentElement.scrollLeft,
      y: document.documentElement.scrollTop,
    }));

    await client.send("Input.dispatchTouchEvent", {
      touchPoints: [{ id: 1, x: point.x, y: point.y }],
      type: "touchStart",
    });
    await waitForAnimationFrame(page);
    for (let step = 1; step <= 4; step += 1) {
      await dispatchTouchMove(client, page, [
        {
          id: 1,
          x: point.x + (50 * step) / 4,
          y: point.y + (40 * step) / 4,
        },
      ]);
    }
    await client.send("Input.dispatchTouchEvent", {
      touchPoints: [],
      type: "touchEnd",
    });
    await page.waitForFunction(
      (before) =>
        document.querySelector(".react-flow__viewport")?.style.transform !==
        before,
      initialTransform,
    );

    const finalScroll = await page.evaluate(() => ({
      x: document.documentElement.scrollLeft,
      y: document.documentElement.scrollTop,
    }));
    if (
      finalScroll.x !== initialScroll.x ||
      finalScroll.y !== initialScroll.y
    ) {
      throw new Error("Graph touch gestures scrolled the page");
    }
  } finally {
    await context.close();
  }
}

async function verifyDesktopPaneSelectionDrag(browser) {
  const { context, page } = await openStory(browser, {
    viewport: { height: 900, width: 1280 },
  });

  try {
    const point = await emptyPanePoint(page);
    const initialTransform = await viewportTransform(page);
    const initialScroll = await page.evaluate(() => ({
      x: document.documentElement.scrollLeft,
      y: document.documentElement.scrollTop,
    }));
    await page.mouse.move(point.x, point.y);
    await page.mouse.down();
    await page.mouse.move(point.x + 80, point.y + 60, { steps: 4 });
    await page.mouse.up();
    await waitForAnimationFrame(page);
    const finalTransform = await viewportTransform(page);
    if (finalTransform !== initialTransform) {
      throw new Error("Primary-button selection drag panned the graph");
    }
    const finalScroll = await page.evaluate(() => ({
      x: document.documentElement.scrollLeft,
      y: document.documentElement.scrollTop,
    }));
    if (
      finalScroll.x !== initialScroll.x ||
      finalScroll.y !== initialScroll.y
    ) {
      throw new Error("Desktop graph selection scrolled the page");
    }
  } finally {
    await context.close();
  }
}

const browser = await chromium.launch({ headless: true });
try {
  await verifyTouchGestures(browser);
  await verifyDesktopPaneSelectionDrag(browser);
  console.log(
    "Factory graph touch-pan and desktop selection-drag verification passed.",
  );
} finally {
  await browser.close();
}
