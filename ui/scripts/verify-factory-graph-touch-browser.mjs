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

async function verifyMixedWorkstationSemantics(page) {
  const viewportTransformValue = await viewportTransform(page);
  if (!/scale\([^)]*\)/.test(viewportTransformValue)) {
    throw new Error(
      `Factory graph did not settle at fit-to-view zoom: ${viewportTransformValue}`,
    );
  }

  const expectedWorkstations = [
    ["Classifier route", "Classifier"],
    ["Logical route", "Logical move"],
    [
      "Inference workstation with a deliberately long authored title",
      "Inference",
    ],
    ["Agent worker", "Agent"],
    ["execute-goal", "Repeater"],
    ["Script cron", "Cron"],
    ["Poller source", "Poller"],
  ];

  for (const [name, semanticLabel] of expectedWorkstations) {
    const button = page.getByRole("button", {
      name: `Select ${name} workstation`,
    });
    await button.waitFor({ state: "visible" });
    await button
      .locator(
        "[data-workstation-runtime-label], [data-workstation-scheduling-label]",
        { hasText: semanticLabel },
      )
      .first()
      .waitFor({
        state: "visible",
      });
  }

  const guardCard = page.locator("[data-workstation-guard-card]");
  await guardCard.waitFor({ state: "visible" });
  const guardText = await guardCard.textContent();
  if (
    !guardText?.includes("Loop breaker") ||
    !guardText.includes("execute-goal") ||
    !guardText.includes("3")
  ) {
    throw new Error(
      `Guarded workstation card did not expose its authored target and limit: ${guardText ?? "<empty>"}`,
    );
  }

  const guardRows = guardCard.locator("[data-workstation-guard-row]");
  if ((await guardRows.count()) !== 2) {
    throw new Error(
      `Expected loop-breaker details to have two single-line rows, found ${await guardRows.count()}`,
    );
  }

  const guardNode = guardCard.locator(
    "xpath=ancestor::*[contains(concat(' ', normalize-space(@class), ' '), ' react-flow__node ')][1]",
  );
  const guardNodeBounds = await guardNode.boundingBox();
  const guardCardBounds = await guardCard.boundingBox();
  if (!guardNodeBounds || !guardCardBounds) {
    throw new Error("Could not measure the loop-breaker node and guard card");
  }
  if (
    guardCardBounds.x < guardNodeBounds.x ||
    guardCardBounds.y < guardNodeBounds.y ||
    guardCardBounds.x + guardCardBounds.width >
      guardNodeBounds.x + guardNodeBounds.width ||
    guardCardBounds.y + guardCardBounds.height >
      guardNodeBounds.y + guardNodeBounds.height
  ) {
    throw new Error(
      `Loop-breaker card escaped its workstation node: card=${JSON.stringify(guardCardBounds)} node=${JSON.stringify(guardNodeBounds)}`,
    );
  }

  const guardRowStyles = await guardCard
    .locator("[data-workstation-guard-target], [data-workstation-guard-limit]")
    .evaluateAll((values) =>
      values.map((value) => {
        const style = getComputedStyle(value);
        return {
          overflow: style.overflow,
          textOverflow: style.textOverflow,
          whiteSpace: style.whiteSpace,
        };
      }),
    );
  if (
    guardRowStyles.some(
      (style) =>
        style.overflow !== "hidden" ||
        style.textOverflow !== "ellipsis" ||
        style.whiteSpace !== "nowrap",
    )
  ) {
    throw new Error(
      `Loop-breaker detail values are not single-line truncating fields: ${JSON.stringify(guardRowStyles)}`,
    );
  }

  const loopNodeStyle = await guardNode.getAttribute("style");
  if (!loopNodeStyle?.includes("height: 156px")) {
    throw new Error(
      `Expected the default loop-breaker workstation height to be 156px: ${loopNodeStyle ?? "<missing>"}`,
    );
  }
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
    await verifyMixedWorkstationSemantics(page);
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
    await verifyMixedWorkstationSemantics(page);
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
