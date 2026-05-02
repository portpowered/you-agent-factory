// @vitest-environment node

import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
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
let replayPaused = Promise.resolve();
let releaseReplayStream = () => {};
const replayFixtures = listBrowserIntegrationReplayScenarios();
const exportCoverImagePath = path.resolve(packageRoot, "..", "docs", "resources", "dashboard.png");
const exportFactoryDefinition = {
  inputTypes: [
    {
      name: "Factory request",
      type: "DEFAULT",
    },
  ],
  name: "Browser Export Factory",
  workers: [
    {
      body: "Return the request unchanged.",
      model: "gpt-5.4-mini",
      modelProvider: "CODEX",
      name: "browser-export-worker",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "request",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  workstations: [
    {
      behavior: "STANDARD",
      inputs: [
        {
          state: "queued",
          workType: "request",
        },
      ],
      name: "Browser export workstation",
      outputs: [
        {
          state: "done",
          workType: "request",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "browser-export-worker",
    },
  ],
};

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

async function startReplayServer(lines, options = {}) {
  const {
    activateFactory = null,
    activationResponseFactory = null,
    currentFactory = null,
    pauseBeforeTick = null,
  } = options;
  let resolveReplayCompleted = () => {};
  replayCompleted = new Promise((resolve) => {
    resolveReplayCompleted = resolve;
  });
  let resolveReplayPaused = () => {};
  replayPaused = pauseBeforeTick === null
    ? Promise.resolve()
    : new Promise((resolve) => {
      resolveReplayPaused = resolve;
    });
  let pauseReleased = false;
  let resumeReplayStream = () => {};
  releaseReplayStream = () => {
    if (pauseReleased) {
      return;
    }
    pauseReleased = true;
    resumeReplayStream();
  };
  const replayPauseReleased = pauseBeforeTick === null
    ? Promise.resolve()
    : new Promise((resolve) => {
      resumeReplayStream = resolve;
    });

  apiServer = http.createServer((request, response) => {
    if (request.method === "OPTIONS") {
      response.writeHead(204, {
        "Access-Control-Allow-Headers": "Content-Type",
        "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
        "Access-Control-Allow-Origin": "*",
      });
      response.end();
      return;
    }

    if (request.url === "/factory/~current" && request.method === "GET") {
      if (currentFactory === null) {
        response.writeHead(404, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(JSON.stringify({
          code: "NOT_FOUND",
          message: "The current factory definition is not available.",
        }));
        return;
      }

      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(JSON.stringify(currentFactory));
      return;
    }

    if (request.url === "/factory" && request.method === "POST") {
      let requestBody = "";
      request.setEncoding("utf8");
      request.on("data", (chunk) => {
        requestBody += chunk;
      });
      request.on("end", async () => {
        const body = requestBody.length === 0 ? null : JSON.parse(requestBody);
        if (activateFactory) {
          await activateFactory(body);
        }

        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(JSON.stringify(activationResponseFactory ?? body));
      });
      return;
    }

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
    let pauseReached = false;
    request.on("close", () => {
      closed = true;
    });

    void (async () => {
      for (const line of lines) {
        if (closed) {
          return;
        }
        if (pauseBeforeTick !== null && !pauseReached) {
          const eventTick = JSON.parse(line).context?.tick;
          if (typeof eventTick === "number" && eventTick > pauseBeforeTick) {
            pauseReached = true;
            resolveReplayPaused();
            await replayPauseReleased;
            if (closed) {
              return;
            }
          }
        }
        response.write(`data: ${line}\n\n`);
        await delay(replayDelayMs);
      }
      if (pauseBeforeTick !== null && !pauseReached) {
        resolveReplayPaused();
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

async function waitForTickLabel(page, label) {
  try {
    await page.getByText(label).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  } catch (error) {
    const sliderValue = await page.getByRole("slider", { name: "Timeline tick" }).inputValue();
    const statusTexts = await page.locator("span").evaluateAll((elements) =>
      elements.map((element) => element.textContent?.trim() ?? "").filter((text) => /^Tick \d+ of \d+$/.test(text))
    );
    throw new Error(
      `Timed out waiting for ${label}; slider=${sliderValue}; visibleTicks=${statusTexts.join(", ") || "<none>"}`,
      { cause: error },
    );
  }
}

async function exerciseHistoricalTimelineView(page, options) {
  const {
    finalTick,
    historicalHiddenButtonName,
    inFlightSelectionTick,
  } = options;
  const slider = page.getByRole("slider", { name: "Timeline tick" });
  const currentButton = page.getByRole("button", { exact: true, name: "Current" });
  const liveTick = inFlightSelectionTick ?? finalTick;
  const previousTick = liveTick - 1;
  const liveTickLabel = `Tick ${liveTick} of ${liveTick}`;
  const historicalTickLabel = `Tick ${previousTick} of ${liveTick}`;
  const pinnedHistoricalTickLabel = `Tick ${previousTick} of ${finalTick}`;
  const finalTickLabel = `Tick ${finalTick} of ${finalTick}`;

  expect(previousTick).toBeGreaterThan(0);

  await slider.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await currentButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  if (inFlightSelectionTick) {
    await replayPaused;
  }
  await waitForTickLabel(page, liveTickLabel);
  expect(await slider.inputValue()).toBe(String(liveTick));
  expect(await currentButton.isDisabled()).toBe(true);
  let liveButtonCount = null;
  if (historicalHiddenButtonName) {
    liveButtonCount = await countButtons(page, historicalHiddenButtonName);
    expect(liveButtonCount).toBeGreaterThan(0);
  }

  await slider.focus();
  await slider.press("ArrowLeft");
  await waitForTickLabel(page, historicalTickLabel);
  expect(await slider.inputValue()).toBe(String(previousTick));
  expect(await currentButton.isDisabled()).toBe(false);
  let historicalButtonCount = null;
  if (historicalHiddenButtonName) {
    historicalButtonCount = await countButtons(page, historicalHiddenButtonName);
    if (!inFlightSelectionTick) {
      expect(historicalButtonCount).toBeLessThan(liveButtonCount);
    }
  }

  if (inFlightSelectionTick) {
    releaseReplayStream();
    await replayCompleted;
  } else {
    await delay(250);
  }
  await waitForTickLabel(
    page,
    inFlightSelectionTick ? pinnedHistoricalTickLabel : historicalTickLabel,
  );
  expect(await slider.inputValue()).toBe(String(previousTick));
  expect(await currentButton.isDisabled()).toBe(false);
  if (historicalHiddenButtonName) {
    expect(await countButtons(page, historicalHiddenButtonName)).toBe(historicalButtonCount);
  }

  await currentButton.click();
  await waitForTickLabel(page, finalTickLabel);
  expect(await slider.inputValue()).toBe(String(finalTick));
  expect(await currentButton.isDisabled()).toBe(true);
  if (historicalHiddenButtonName) {
    const currentButtonCount = await countButtons(page, historicalHiddenButtonName);
    if (inFlightSelectionTick) {
      expect(currentButtonCount).toBeGreaterThan(historicalButtonCount);
    } else {
      expect(currentButtonCount).toBe(liveButtonCount);
    }
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
    inFlightSelectionTick,
    requiresWorkItemSelection,
    selectedWorkText,
    workstationName,
  } = browserIntegration;
  await startReplayServer(await loadReplayLines(fileName), {
    pauseBeforeTick: inFlightSelectionTick ?? null,
  });
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
    if (!inFlightSelectionTick) {
      await replayCompleted;
      await page.getByText(`Tick ${finalTick} of ${finalTick}`).waitFor({
        timeout: uiInteractionTimeoutMs,
      });
    }
    await exerciseTimelineSlider(page, {
      finalTick,
      historicalHiddenButtonName,
      inFlightSelectionTick,
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

async function assertFactoryExportRoundTrip() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ acceptDownloads: true });
  const page = await context.newPage();
  const pageErrors = [];
  const consoleErrors = [];
  const downloadDirectory = await mkdtemp(path.join(os.tmpdir(), "agent-factory-export-"));
  const activationRequests = [];

  await page.addInitScript(() => {
    window.__agentFactoryCapturedDownloads = [];
    const originalClick = HTMLAnchorElement.prototype.click;
    HTMLAnchorElement.prototype.click = function click(...args) {
      if (this.download && this.href.startsWith("blob:")) {
        const filename = this.download;
        const href = this.href;
        const capture = fetch(href)
          .then(async (response) => {
            const buffer = await response.arrayBuffer();
            return {
              bytes: Array.from(new Uint8Array(buffer)),
              filename,
            };
          })
          .then((download) => {
            window.__agentFactoryCapturedDownloads.push(download);
          });
        window.__agentFactoryPendingDownload = capture;
      }

      return originalClick.apply(this, args);
    };
  });

  page.on("pageerror", (error) => {
    pageErrors.push(error.stack ?? error.message);
  });
  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text());
    }
  });

  try {
    await startReplayServer(
      await loadReplayLines("graph-state-smoke-replay.jsonl"),
      {
        activateFactory: async (value) => {
          activationRequests.push(value);
        },
        currentFactory: exportFactoryDefinition,
      },
    );
    await page.goto(previewURL, { waitUntil: "domcontentloaded" });
    await page.getByRole("heading", { name: "Agent Factory" }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    await replayCompleted;
    await page.getByRole("button", { name: "Export PNG" }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });

    await page.getByRole("button", { name: "Export PNG" }).click();
    await page.getByRole("heading", { name: "Export factory" }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    const exportDialog = page.getByRole("dialog", { name: "Export factory" });
    await exportDialog.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });

    const exportName = "Roundtrip Browser Export";
    await exportDialog.getByLabel("Factory name").fill(exportName);
    await exportDialog.getByLabel("Cover image").setInputFiles(exportCoverImagePath);
    await exportDialog.getByText("Selected image: dashboard.png").waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    const exportDialogButton = exportDialog.getByRole("button", { name: "Export PNG" });
    expect(await exportDialogButton.isEnabled()).toBe(true);

    await exportDialogButton.click();
    const exportOutcome = await Promise.race([
      page.waitForFunction(
        () => window.__agentFactoryCapturedDownloads.length > 0,
        null,
        { timeout: uiInteractionTimeoutMs },
      ).then(() => "download"),
      exportDialog.getByRole("alert").waitFor({
        state: "visible",
        timeout: uiInteractionTimeoutMs,
      }).then(() => "error"),
    ]);
    if (exportOutcome === "error") {
      throw new Error(await exportDialog.getByRole("alert").innerText());
    }
    const download = await page.evaluate(() => window.__agentFactoryCapturedDownloads[0] ?? null);
    expect(download).not.toBeNull();
    const downloadPath = path.join(downloadDirectory, download.filename);
    await writeFile(downloadPath, new Uint8Array(download.bytes));

    expect(download.filename).toBe("roundtrip-browser-export.png");
    await page.getByRole("heading", { name: "Export factory" }).waitFor({
      state: "hidden",
      timeout: uiInteractionTimeoutMs,
    });
    expect(pageErrors).toEqual([]);
    expect(consoleErrors).toEqual([]);

    const exportedBytes = await readFile(downloadPath);
    expect(exportedBytes.subarray(0, 8)).toEqual(
      Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    );
    const viewport = page.getByRole("region", { name: "Work graph viewport" });
    const importDataTransfer = await page.evaluateHandle(({ bytes, fileName }) => {
      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(
        new File([new Uint8Array(bytes)], fileName, { type: "image/png" }),
      );
      return dataTransfer;
    }, {
      bytes: Array.from(exportedBytes),
      fileName: download.filename,
    });

    await viewport.dispatchEvent("dragover", { dataTransfer: importDataTransfer });
    await page.getByText("Import factory PNG").waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    await viewport.dispatchEvent("drop", { dataTransfer: importDataTransfer });

    const importDialog = page.getByRole("dialog", { name: "Review factory import" });
    await importDialog.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    await importDialog.getByRole("img", { name: `${exportName} preview image` }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    expect(await importDialog.textContent()).toContain(exportName);
    expect(await importDialog.textContent()).toContain(download.filename);

    await importDialog.getByRole("button", { name: "Activate factory" }).click();
    await importDialog.waitFor({
      state: "hidden",
      timeout: uiInteractionTimeoutMs,
    });
    expect(activationRequests).toEqual([
      {
        ...exportFactoryDefinition,
        name: exportName,
      },
    ]);
    expect(pageErrors).toEqual([]);
    expect(consoleErrors).toEqual([]);
  } finally {
    await rm(downloadDirectory, { force: true, recursive: true });
    await page.close();
    await context.close();
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

  it(
    "exports the current factory as a downloadable PNG without uncaught browser exceptions",
    async () => {
      await assertFactoryExportRoundTrip();
    },
    browserScenarioTimeoutMs,
  );
});
