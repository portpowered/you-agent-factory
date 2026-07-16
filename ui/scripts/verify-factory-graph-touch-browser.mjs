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
    for (let y = bounds.top + 80; y < bounds.bottom - 20; y += 40) {
      for (let x = bounds.left + 20; x < bounds.right - 20; x += 40) {
        if (document.elementFromPoint(x, y) === pane) {
          return { x, y };
        }
      }
    }
    return null;
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

async function viewportZoom(page) {
  return page.locator(".react-flow__viewport").evaluate((viewport) => {
    const matrix = new DOMMatrix(getComputedStyle(viewport).transform);
    return matrix.a;
  });
}

async function verifyTouchGestures(browser) {
  const { context, page } = await openStory(browser, {
    hasTouch: true,
    isMobile: true,
    viewport: { height: 812, width: 375 },
  });

  try {
    const client = await context.newCDPSession(page);
    const nodeButtons = page.locator(
      '.react-flow__node button[aria-label^="Select "]',
    );
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
    await client.send("Input.dispatchTouchEvent", {
      touchPoints: [{ id: 1, x: point.x + 50, y: point.y + 40 }],
      type: "touchMove",
    });
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

    const zoomBeforePinch = await viewportZoom(page);
    await client.send("Input.dispatchTouchEvent", {
      touchPoints: [
        { id: 2, x: point.x + 20, y: point.y },
        { id: 3, x: point.x + 60, y: point.y },
      ],
      type: "touchStart",
    });
    await client.send("Input.dispatchTouchEvent", {
      touchPoints: [
        { id: 2, x: point.x, y: point.y },
        { id: 3, x: point.x + 80, y: point.y },
      ],
      type: "touchMove",
    });
    await client.send("Input.dispatchTouchEvent", {
      touchPoints: [],
      type: "touchEnd",
    });
    await page.waitForFunction((before) => {
      const viewport = document.querySelector(".react-flow__viewport");
      return viewport
        ? new DOMMatrix(getComputedStyle(viewport).transform).a > before
        : false;
    }, zoomBeforePinch);

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

async function verifyDesktopSelectionAndKeyboardPan(browser) {
  const { context, page } = await openStory(browser, {
    viewport: { height: 900, width: 1280 },
  });

  try {
    const point = await emptyPanePoint(page);
    const initialTransform = await viewportTransform(page);
    await page.mouse.move(point.x, point.y);
    await page.mouse.down();
    await page.mouse.move(point.x + 80, point.y + 60, { steps: 4 });
    await page.locator(".react-flow__selection").waitFor({ state: "visible" });
    await page.mouse.up();
    if ((await viewportTransform(page)) !== initialTransform) {
      throw new Error("Primary mouse selection unexpectedly panned the graph");
    }

    await page.keyboard.down("Space");
    await page.mouse.move(point.x, point.y);
    await page.mouse.down();
    await page.mouse.move(point.x + 50, point.y + 30, { steps: 3 });
    await page.mouse.up();
    await page.keyboard.up("Space");
    await page.waitForFunction(
      (before) =>
        document.querySelector(".react-flow__viewport")?.style.transform !==
        before,
      initialTransform,
    );
  } finally {
    await context.close();
  }
}

const browser = await chromium.launch({ headless: true });
try {
  await verifyTouchGestures(browser);
  await verifyDesktopSelectionAndKeyboardPan(browser);
  console.log("Factory graph touch and desktop gesture verification passed.");
} finally {
  await browser.close();
}
