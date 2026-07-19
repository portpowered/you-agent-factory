import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";
import { verifyDenseTopologyChromeModes } from "./verify-topology-chrome-modes.mjs";

const require = createRequire(import.meta.url);
const axePath = require.resolve("axe-core/axe.min.js");

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
  await verifyRecordingComposition(browser);
  await verifyRecordingPresentations(browser);
  await verifyGermanFormatting(browser);
  await verifyReducedMotion(browser);
  console.log(
    "Factory visualizer accessibility and responsive browser checks passed.",
  );
} finally {
  await browser?.close();
  server.kill();
}

async function verifyRecordingPresentations(browserInstance) {
  const context = await browserInstance.newContext({
    viewport: { width: 1200, height: 900 },
  });
  const page = await context.newPage();
  const stories = [
    "factory-visualizers-factoryrecordingtopologyreplay--same-tick-history-and-current",
    "factory-visualizers-factoryrecordingtopologyreplay--dense-recording",
    "factory-visualizers-factoryrecordingtopologyreplay--annotated-recording",
    "factory-visualizers-factoryrecordingtopologyreplay--localized-recording",
    "factory-visualizers-factorytopologyreplay--emulator-ready-dense-annotations",
  ];
  for (const storyId of stories) {
    await openStory(page, storyId);
    await assertNoPageOverflow(page, storyId);
    await assertNoSeriousAccessibilityViolations(page, storyId);
  }

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--same-tick-history-and-current",
  );
  const sameTickSlider = page.getByRole("slider", {
    name: "Select recording tick",
  });
  await tabTo(page, sameTickSlider, 30);
  await page.keyboard.press("ArrowRight");
  await page
    .getByRole("button", { name: "workstation: triage" })
    .getByText("1 active Dispatch")
    .waitFor({ state: "visible", timeout: 5_000 });

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--dense-recording",
  );
  const denseNode = page.getByRole("button", {
    name: "workstation: Review",
  });
  await tabTo(page, denseNode, 100);
  await page.keyboard.press("Enter");
  assert(
    (await page.locator(".react-flow__edge").count()) >= 20,
    "The dense recording omitted canonical topology connections.",
  );

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--localized-recording",
  );
  assert(
    await page.getByText("Ausgewählter logischer Schritt 7.000").isVisible(),
    "The localized recording does not format logical ticks through its formatter.",
  );
  assert(
    await page.getByText("2 Aufträge insgesamt").isVisible(),
    "The localized recording does not use localized plural Work copy.",
  );
  await verifyAnnotatedRecording(page);
  await context.close();

  await verifyNarrowRecording(browserInstance);
}

async function verifyAnnotatedRecording(page) {
  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--annotated-recording",
  );
  const showAnnotations = page.getByRole("button", {
    name: "Show annotations",
  });
  const annotationsToggle = page.getByRole("button", {
    name: /^(Show|Hide) annotations$/,
  });
  await annotationsToggle.waitFor({ state: "visible", timeout: 5_000 });
  if (
    await page.getByRole("button", { name: "Hide annotations" }).isVisible()
  ) {
    await annotationsToggle.click();
    await showAnnotations.waitFor({ state: "visible", timeout: 5_000 });
  }
  await showAnnotations.click();
  await page
    .getByRole("button", { name: "Hide annotations" })
    .waitFor({ state: "visible", timeout: 5_000 });
  await page
    .getByText("Escalations are reviewed here.")
    .waitFor({ state: "visible", timeout: 5_000 });
  assert(
    await page.getByText("Escalations are reviewed here.").isVisible(),
    "The annotated recording did not render its caller-owned layout sidecar.",
  );
  await page.getByRole("button", { name: "Hide annotations" }).click();
  assert(
    !(await page.getByText("Escalations are reviewed here.").isVisible()),
    "Hiding annotations in the recording did not remove the annotation node.",
  );
}

async function verifyNarrowRecording(browserInstance) {
  const narrowContext = await browserInstance.newContext({
    viewport: { width: 360, height: 800 },
  });
  const narrowPage = await narrowContext.newPage();
  const narrowStory =
    "factory-visualizers-factoryrecordingtopologyreplay--narrow-viewport";
  await openStory(narrowPage, narrowStory);
  await narrowPage.evaluate(() => {
    document.body.style.zoom = "200%";
  });
  await assertNoPageOverflow(narrowPage, `${narrowStory} at 200% zoom`);
  await assertNoSeriousAccessibilityViolations(narrowPage, narrowStory);
  const narrowSlider = narrowPage.getByRole("slider", {
    name: "Select recording tick",
  });
  await tabTo(narrowPage, narrowSlider, 100);
  await narrowPage.keyboard.press("ArrowLeft");
  assert(
    await narrowPage.getByText("Inspecting recording history").isVisible(),
    "The narrow high-zoom timeline is not keyboard operable.",
  );
  assert(
    await narrowPage
      .getByRole("region", { name: "Recorded Work progress" })
      .isVisible(),
    "The narrow high-zoom Work progress region is unreachable.",
  );
  await narrowContext.close();
}

async function verifyRecordingComposition(browserInstance) {
  const context = await browserInstance.newContext({
    viewport: { width: 720, height: 900 },
  });
  const page = await context.newPage();
  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--loading",
  );
  const loadingRegion = page.getByRole("region", {
    name: "Recorded Factory topology",
  });
  assert(
    (await loadingRegion.getAttribute("aria-busy")) === "true",
    "The recording loading story does not expose an accessible busy state.",
  );

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--empty-recording",
  );
  assert(
    await page
      .getByText("No Factory topology is available at this tick.")
      .isVisible(),
    "The empty recording story does not expose an intentional empty topology state.",
  );

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--validated-recording",
  );
  assert(
    await page
      .getByRole("region", { name: "Recorded Factory playback" })
      .isVisible(),
    "The validated recording composition is not a visible labeled region.",
  );
  const workstation = page.getByRole("button", {
    name: "workstation: triage",
  });
  await tabTo(page, workstation, 50);
  assert(
    (await workstation.evaluate(
      (element) => getComputedStyle(element).outlineWidth,
    )) !== "0px",
    "The validated recording topology does not expose visible keyboard focus.",
  );

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--same-tick-history-and-current",
  );
  const slider = page.getByRole("slider", { name: "Select recording tick" });
  await tabTo(page, slider, 20);
  await page.keyboard.press("ArrowLeft");
  assert(
    await page.getByText("Tick 1 of 3").isVisible(),
    "Keyboard scrubbing fabricated a sparse tick instead of selecting recorded history.",
  );
  assert(
    await page.getByText("Inspecting recording history").isVisible(),
    "The recording did not expose visible history status after keyboard scrubbing.",
  );
  assert(
    await slider.evaluate((element) => element === document.activeElement),
    "Keyboard scrubbing did not preserve focus on the recording timeline.",
  );
  await page.getByRole("button", { name: "Follow latest" }).click();
  assert(
    await page.getByText("Following current recording").isVisible(),
    "Following latest did not return recording playback to current mode.",
  );

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--invalid-recording",
  );
  assert(
    await page.getByRole("alert").isVisible(),
    "The invalid recording does not render the shared accessible failure.",
  );
  assert(
    await page
      .getByRole("button", { name: "Sibling example control" })
      .isVisible(),
    "The invalid recording made sibling Storybook content unusable.",
  );

  await openStory(
    page,
    "factory-visualizers-factoryrecordingtopologyreplay--projection-failure",
  );
  assert(
    await page.getByRole("alert").isVisible(),
    "The projection failure does not render the shared accessible failure.",
  );
  assert(
    (await page.locator(".react-flow").count()) === 0,
    "The projection failure retained stale ready-state graph content.",
  );
  assert(
    await page
      .getByRole("button", { name: "Sibling example control" })
      .isVisible(),
    "The projection failure made sibling Storybook content unusable.",
  );
  await context.close();
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
    await verifyDenseTopologyChromeModes({
      assert,
      page,
      tabTo,
      verifyLayout,
      width: viewport.width,
    });

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
  await assertNoPageOverflow(page, storyId);
}

async function assertNoPageOverflow(page, storyId) {
  const hasPageOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  );
  assert(!hasPageOverflow, `${storyId} has page-level horizontal overflow.`);
}

async function assertNoSeriousAccessibilityViolations(page, storyId) {
  await page.addScriptTag({ path: axePath });
  const violations = await page.evaluate(async () => {
    const result = await window.axe.run(document, {
      runOnly: {
        type: "tag",
        values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"],
      },
    });
    return result.violations
      .filter(({ impact }) => impact === "critical" || impact === "serious")
      .map(({ id, impact, nodes }) => ({
        id,
        impact,
        nodes: nodes.map(({ failureSummary, target }) => ({
          failureSummary,
          target,
        })),
      }));
  });
  assert(
    violations.length === 0,
    `${storyId} has serious accessibility violations: ${JSON.stringify(violations)}`,
  );
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
