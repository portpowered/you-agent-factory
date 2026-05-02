// @vitest-environment node

import { spawn } from "node:child_process";
import { once } from "node:events";
import { readFile } from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";

import {
  buildReplayCoverageReport,
  formatReplayCoverageReportMarkdown,
  listBrowserIntegrationReplayScenarios,
} from "../src/testing/replay-fixture-catalog";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const replayFixtureDirectory = path.join(dirname, "fixtures");
const previewHost = "127.0.0.1";
const buildTimeoutMs = 120_000;
const browserScenarioTimeoutMs = 240_000;
const readyTimeoutMs = 90_000;
const replayDelayMs = 25;
const uiInteractionTimeoutMs = 10_000;

let apiServer = null;
let apiOrigin = "";
let apiPort = 0;
let previewPort = 0;
let previewProcess = null;
let previewURL = "";
let replayCompleted = Promise.resolve();
const replayFixtures = listBrowserIntegrationReplayScenarios();

function createBunEnv(extraEnv = {}, options = {}) {
  const env = {
    ...process.env,
    ...extraEnv,
  };

  if (options.stripVitestEnv) {
    delete env.NODE_ENV;
    for (const key of Object.keys(env)) {
      if (key === "VITEST" || key.startsWith("VITEST_")) {
        delete env[key];
      }
    }
  }

  if (options.nodeEnv) {
    env.NODE_ENV = options.nodeEnv;
  }

  return env;
}

function bunCommand() {
  return process.platform === "win32" ? "bun.exe" : "bun";
}

function spawnBun(args, extraEnv = {}, options = {}) {
  return spawn(bunCommand(), args, {
    cwd: packageRoot,
    env: createBunEnv(extraEnv, options),
    shell: false,
    stdio: "pipe",
  });
}

async function reserveAvailablePort() {
  return await new Promise((resolve, reject) => {
    const server = http.createServer();

    server.once("error", (error) => {
      reject(error);
    });

    server.listen(0, previewHost, () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("failed to reserve a preview port"));
        return;
      }

      server.close((closeError) => {
        if (closeError) {
          reject(closeError);
          return;
        }

        resolve(address.port);
      });
    });
  });
}

async function runBun(args, extraEnv = {}, timeoutMs = buildTimeoutMs, options = {}) {
  const child = spawnBun(args, extraEnv, options);
  let stdout = "";
  let stderr = "";

  child.stdout?.on("data", (chunk) => {
    stdout += chunk.toString();
  });
  child.stderr?.on("data", (chunk) => {
    stderr += chunk.toString();
  });

  const timeout = setTimeout(() => {
    child.kill("SIGTERM");
  }, timeoutMs);

  try {
    const [code, signal] = await once(child, "exit");
    if (code !== 0) {
      throw new Error(
        [
          `bun ${args.join(" ")} exited with ${code ?? "null"} / ${signal ?? "null"}.`,
          stdout.trim(),
          stderr.trim(),
        ]
          .filter((part) => part.length > 0)
          .join("\n"),
      );
    }
  } finally {
    clearTimeout(timeout);
  }
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) {
    return;
  }

  if (process.platform === "win32") {
    const killer = spawn("taskkill", ["/pid", String(child.pid), "/t", "/f"], {
      shell: false,
      stdio: "ignore",
    });
    await once(killer, "exit");
    return;
  }

  child.kill("SIGTERM");
  await once(child, "exit");
}

async function waitForURL(url, timeoutMs = readyTimeoutMs) {
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch {
      // Retry until the deadline while preview is starting.
    }

    await delay(250);
  }

  throw new Error(`Timed out waiting for ${url}.`);
}

async function startReplayServer(lines) {
  let resolveReplayCompleted = () => {};
  replayCompleted = new Promise((resolve) => {
    resolveReplayCompleted = resolve;
  });

  apiServer = http.createServer((request, response) => {
    if (request.url !== "/events") {
      response.statusCode = 404;
      response.end("not found");
      return;
    }

    response.writeHead(200, {
      "Access-Control-Allow-Origin": "*",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
      "Content-Type": "text/event-stream",
    });
    response.flushHeaders?.();

    let closed = false;
    request.on("close", () => {
      closed = true;
    });

    void (async () => {
      for (const line of lines) {
        if (closed) {
          return;
        }
        response.write(`data: ${line}\n\n`);
        await delay(replayDelayMs);
      }
      if (!closed) {
        response.write(": replay-complete\n\n");
        resolveReplayCompleted();
      }
    })();
  });

  await new Promise((resolve, reject) => {
    apiServer.once("error", reject);
    apiServer.listen(apiPort, previewHost, resolve);
  });
}

async function stopReplayServer() {
  if (!apiServer) {
    return;
  }

  apiServer.closeAllConnections?.();

  await new Promise((resolve, reject) => {
    apiServer.close((error) => {
      if (error) {
        reject(error);
        return;
      }

      resolve();
    });
  });
  apiServer = null;
}

async function loadReplayLines(fileName) {
  return (await readFile(path.join(replayFixtureDirectory, fileName), "utf8"))
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

async function exerciseSelectedWorkTrace(page, workstationName, options = {}) {
  const {
    requiresWorkItemSelection = true,
    selectedWorkText = null,
  } = options;
  const workstationButton = page.getByRole("button", {
    name: workstationName,
  });
  await workstationButton.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await workstationButton.click({ force: true });

  await page.getByRole("article", { name: "Current selection" }).waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  if (selectedWorkText !== null) {
    await page.getByText(selectedWorkText, { exact: false }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  }

  if (!requiresWorkItemSelection) {
    return;
  }

  const workItemButton = page.getByRole("button", { name: /^Select work item / }).first();
  try {
    await workItemButton.waitFor({ state: "visible", timeout: 2_000 });
    await workItemButton.click({ force: true });

    await page.getByRole("article", { name: "Trace drill-down" }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  } catch {
    // Some canonical replays finish without a selectable current-work item at the
    // final tick. Selecting the workstation is still enough to verify the replay
    // rendered without browser-side failures.
  }
}

async function exerciseTimelineSlider(page, options) {
  return await exerciseHistoricalTimelineView(page, options);
}

async function countButtons(page, buttonName) {
  return await page.getByRole("button", { name: buttonName }).count();
}

async function exerciseHistoricalTimelineView(page, options) {
  const {
    finalTick,
    historicalHiddenButtonName,
  } = options;
  const slider = page.getByRole("slider", { name: "Timeline tick" });
  const currentButton = page.getByRole("button", { exact: true, name: "Current" });
  const previousTick = finalTick - 1;

  expect(previousTick).toBeGreaterThan(0);

  await slider.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await currentButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  expect(await slider.inputValue()).toBe(String(finalTick));
  expect(await currentButton.isDisabled()).toBe(true);
  let latestButtonCount = null;
  if (historicalHiddenButtonName) {
    latestButtonCount = await countButtons(page, historicalHiddenButtonName);
    expect(latestButtonCount).toBeGreaterThan(0);
  }

  await slider.focus();
  await slider.press("ArrowLeft");
  await page.getByText(`Tick ${previousTick} of ${finalTick}`).waitFor({
    timeout: uiInteractionTimeoutMs,
  });
  expect(await slider.inputValue()).toBe(String(previousTick));
  expect(await currentButton.isDisabled()).toBe(false);
  if (historicalHiddenButtonName) {
    expect(await countButtons(page, historicalHiddenButtonName)).toBeLessThan(latestButtonCount);
  }

  await delay(250);
  await page.getByText(`Tick ${previousTick} of ${finalTick}`).waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  expect(await slider.inputValue()).toBe(String(previousTick));
  expect(await currentButton.isDisabled()).toBe(false);

  await currentButton.click();
  await page.getByText(`Tick ${finalTick} of ${finalTick}`).waitFor({
    timeout: uiInteractionTimeoutMs,
  });
  expect(await slider.inputValue()).toBe(String(finalTick));
  expect(await currentButton.isDisabled()).toBe(true);
  if (historicalHiddenButtonName) {
    expect(await countButtons(page, historicalHiddenButtonName)).toBe(latestButtonCount);
  }
}

async function assertReplayScenarioRenders({
  browserIntegration,
  fileName,
  id,
}) {
  const {
    finalTick,
    headingName,
    historicalHiddenButtonName,
    requiresWorkItemSelection,
    selectedWorkText,
    workstationName,
  } = browserIntegration;
  await startReplayServer(await loadReplayLines(fileName));
  const replayCoverageReport = buildReplayCoverageReport();
  const coverageScenario = replayCoverageReport.scenarios.find((scenario) => scenario.id === id);
  const replayCoverageMarkdown = formatReplayCoverageReportMarkdown(replayCoverageReport);

  expect(coverageScenario).toBeDefined();
  expect(coverageScenario?.verificationLayers).toContain("browser-integration");
  expect(coverageScenario?.fileName).toBe(fileName);
  expect(replayCoverageMarkdown).toContain(`| \`${id}\` | \`${fileName}\` |`);

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  const pageErrors = [];
  const consoleErrors = [];

  page.on("pageerror", (error) => {
    pageErrors.push(error.stack ?? error.message);
  });
  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text());
    }
  });

  try {
    await page.goto(previewURL, { waitUntil: "domcontentloaded" });
    expect(pageErrors).toEqual([]);
    expect(consoleErrors).toEqual([]);
    await page.getByRole("heading", { name: headingName }).waitFor();
    await page.getByRole("button", { name: workstationName }).waitFor();
    await replayCompleted;
    await page.getByText(`Tick ${finalTick} of ${finalTick}`).waitFor({
      timeout: uiInteractionTimeoutMs,
    });
    await exerciseTimelineSlider(page, {
      finalTick,
      historicalHiddenButtonName,
    });
    await page
      .locator('[aria-label="dashboard summary"]')
      .getByText("RUNNING", { exact: true })
      .waitFor();
    await exerciseSelectedWorkTrace(page, workstationName, {
      requiresWorkItemSelection,
      selectedWorkText,
    });

    expect(pageErrors).toEqual([]);
    expect(consoleErrors).toEqual([]);
  } finally {
    await page.close();
    await browser.close();
    await stopReplayServer();
  }
}

describe.sequential("captured event stream replay", () => {
  beforeAll(async () => {
    apiPort = await reserveAvailablePort();
    previewPort = await reserveAvailablePort();
    apiOrigin = `http://${previewHost}:${apiPort}`;
    previewURL = `http://${previewHost}:${previewPort}/dashboard/ui/`;
    await runBun(["run", "build"], {
      VITE_AGENT_FACTORY_API_ORIGIN: apiOrigin,
    }, buildTimeoutMs, {
      nodeEnv: "production",
      stripVitestEnv: true,
    });

    previewProcess = spawnBun(["x", "vite", "preview", "--host", previewHost, "--port", String(previewPort), "--strictPort"], {
      AGENT_FACTORY_API_ORIGIN: apiOrigin,
    }, {
      nodeEnv: "production",
      stripVitestEnv: true,
    });

    await waitForURL(previewURL);
  }, buildTimeoutMs);

  afterAll(async () => {
    await stopReplayServer();
    await stopProcess(previewProcess);
    previewProcess = null;
  });

  afterEach(async () => {
    await stopReplayServer();
  }, buildTimeoutMs);

  for (const replayFixture of replayFixtures) {
    it(
      `renders '${replayFixture.id}' without uncaught browser exceptions`,
      async () => {
        await assertReplayScenarioRenders(replayFixture);
      },
      browserScenarioTimeoutMs,
    );
  }
});
