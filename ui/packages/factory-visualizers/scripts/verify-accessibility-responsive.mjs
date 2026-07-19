import { spawn } from "node:child_process";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const port = Number.parseInt(
  process.env.FACTORY_VISUALIZERS_STORYBOOK_PORT ?? "3767",
  10,
);
const baseUrl = `http://127.0.0.1:${port}`;
const httpServer = path.resolve(
  packageRoot,
  "../../node_modules/http-server/bin/http-server",
);

if (!Number.isSafeInteger(port) || port < 3100 || port > 3999) {
  throw new Error(
    "FACTORY_VISUALIZERS_STORYBOOK_PORT must be an integer from 3100 to 3999.",
  );
}

await assertPortAvailable(port);
const server = spawn(
  process.execPath,
  [httpServer, "storybook-static", "-p", String(port), "-a", "127.0.0.1", "-s"],
  { cwd: packageRoot, stdio: "ignore", windowsHide: true },
);
let browser;

try {
  await waitForServer();
  browser = await chromium.launch({ headless: true });
  await verifyResponsiveViewports(browser);
  await verifyGermanFormatting(browser);
  await verifyReducedMotion(browser);
  console.log(
    "Factory visualizer accessibility and responsive browser checks passed.",
  );
} finally {
  await browser?.close();
  server.kill();
}

async function verifyResponsiveViewports(browserInstance) {
  const viewports = [
    { width: 360, height: 800, progress: "small" },
    { width: 720, height: 900, progress: "medium" },
    { width: 1200, height: 900, progress: "large" },
  ];

  for (const viewport of viewports) {
    const context = await browserInstance.newContext({ viewport });
    const page = await context.newPage();
    await verifyLayout(
      page,
      `factory-visualizers-workprogressvisualizer--${viewport.progress}`,
    );
    await verifyLayout(
      page,
      "factory-visualizers-factorytopologyreplay--dense-prepared-projection",
    );
    assert(
      await page.locator(".react-flow__controls").isVisible(),
      `Dense topology controls are hidden at ${viewport.width}px.`,
    );

    const workstation = page.getByRole("button", {
      name: "workstation: Review",
    });
    await tabTo(page, workstation, 50);
    assert(
      (await workstation.evaluate(
        (element) => getComputedStyle(element).outlineWidth,
      )) !== "0px",
      `Topology node focus is not visible at ${viewport.width}px.`,
    );
    await page.keyboard.press("Enter");
    const zoomIn = page.getByRole("button", { name: "Zoom in" });
    await tabTo(page, zoomIn, 50);
    assert(
      (await zoomIn.evaluate(
        (element) => getComputedStyle(element).outlineStyle,
      )) !== "none",
      `Graph control focus is not visible at ${viewport.width}px.`,
    );

    await verifyLayout(
      page,
      "factory-visualizers-factorytimelinescrubber--history-keyboard",
    );
    const slider = page.getByRole("slider", { name: "Select replay tick" });
    await tabTo(page, slider, 10);
    assert(
      (await slider.evaluate(
        (element) => getComputedStyle(element).outlineWidth,
      )) !== "0px",
      `Timeline focus is not visible at ${viewport.width}px.`,
    );
    await page.keyboard.press("ArrowRight");
    assert(
      await page.getByText("Tick 8 of 24").isVisible(),
      `Timeline selection stopped being host-controlled at ${viewport.width}px.`,
    );
    await page.keyboard.press("Tab");
    assert(
      await page
        .getByRole("button", { name: "Follow latest" })
        .evaluate((element) => element === document.activeElement),
      `Timeline actions are not in logical keyboard order at ${viewport.width}px.`,
    );

    await verifyLayout(
      page,
      "factory-visualizers-factorytopologyreplay--failed-with-retry",
    );
    const retry = page.getByRole("button", { name: "Try again" });
    assert(
      await retry.isVisible(),
      `The failure retry action is hidden at ${viewport.width}px.`,
    );
    await tabTo(page, retry, 5);
    await context.close();
  }
}

async function verifyGermanFormatting(browserInstance) {
  const context = await browserInstance.newContext({
    viewport: { width: 720, height: 900 },
  });
  const page = await context.newPage();
  await openStory(
    page,
    "factory-visualizers-factorytimelinescrubber--german-history",
  );
  assert(
    await page.getByText("Schritt 7.000 von 12.000").isVisible(),
    "The German tick formatter evidence is missing.",
  );
  await context.close();
}

async function verifyReducedMotion(browserInstance) {
  const context = await browserInstance.newContext({
    reducedMotion: "reduce",
    viewport: { width: 720, height: 900 },
  });
  const page = await context.newPage();
  await openStory(
    page,
    "factory-visualizers-factorytopologyreplay--dense-prepared-projection",
  );
  const topology = page.getByRole("region", {
    name: "Factory topology at selected tick",
  });
  assert(
    (await topology.getAttribute("data-reduced-motion")) === "true",
    "The reduced-motion preference was not observed.",
  );
  assert(
    (await page.locator(".react-flow__edge.animated").count()) === 0,
    "An active topology edge remained animated.",
  );
  await context.close();
}

async function verifyLayout(page, storyId) {
  await openStory(page, storyId);
  const hasPageOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  );
  assert(!hasPageOverflow, `${storyId} has page-level horizontal overflow.`);
}

async function openStory(page, storyId) {
  await page.goto(`${baseUrl}/iframe.html?id=${storyId}&viewMode=story`, {
    timeout: 10_000,
    waitUntil: "domcontentloaded",
  });
  try {
    await page.getByRole("region").first().waitFor({ timeout: 10_000 });
  } catch (error) {
    const body = (await page.locator("body").innerText()).slice(0, 800);
    throw new Error(`${storyId} did not render a labeled region. ${body}`, {
      cause: error,
    });
  }
}

async function tabTo(page, locator, attempts) {
  for (let index = 0; index < attempts; index++) {
    if (await locator.evaluate((element) => element === document.activeElement))
      return;
    await page.keyboard.press("Tab");
  }
  throw new Error(
    "The target control was not reachable in the expected keyboard order.",
  );
}

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt++) {
    try {
      const response = await fetch(`${baseUrl}/index.json`, {
        signal: AbortSignal.timeout(2_000),
      });
      if (response.ok) return;
    } catch {
      // The bounded retry loop reports one failure if the server never becomes ready.
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error("The temporary Storybook server did not become ready.");
}

async function assertPortAvailable(candidatePort) {
  await new Promise((resolve, reject) => {
    const probe = net.createServer();
    probe.once("error", () =>
      reject(new Error(`Port ${candidatePort} is already occupied.`)),
    );
    probe.once("listening", () => probe.close(resolve));
    probe.listen(candidatePort, "127.0.0.1");
  });
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}
