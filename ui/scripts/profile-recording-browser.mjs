// biome-ignore lint/style/noExcessiveLinesPerFile: Standalone diagnostic CLI keeps setup, serving, probing, and reporting together.
import { spawn } from "node:child_process";
import { once } from "node:events";
import { access, mkdir, readFile, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

import {
  defaultFactorySessionID,
  openBrowserPage,
  startBrowserPreview,
  uiInteractionTimeoutMs,
} from "../integration/browser-test-harness.mjs";

const { SourceMapConsumer } = await import("source-map");

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const promptTemplateContractPathPattern =
  /^\/factory-sessions\/[^/]+\/factory\/workstations\/[^/]+\/prompt-template-contract$/;
const promptTemplateValidationPathPattern =
  /^\/factory-sessions\/[^/]+\/factory\/workstations\/[^/]+\/prompt-template-validation$/;
const correctedDefaultRecordingPath = path.join(
  os.homedir(),
  ".you-agent-factory",
  "recordings",
  "2026-06",
  "2026-06-21",
  "factory-session-~default-171847-f3b2eecc-b403-4386-832f-ce14eff555ac.json",
);

function parseArgs(argv) {
  const options = {
    apiPort: null,
    artifactDir: path.join(packageRoot, "tmp", "recording-browser-profile"),
    burstSize: 500,
    compactEventText: false,
    delayMs: 0,
    disableCheckpointPersistence: false,
    heapGC: false,
    heapProfile: false,
    heapProfileSamplingInterval: 32768,
    heapProfileTopN: 20,
    inputPath: correctedDefaultRecordingPath,
    json: false,
    maxEventTextChars: null,
    maxEvents: null,
    reportPath: null,
    reuseBuild: false,
    settleMs: 2_000,
    sourceMaps: false,
    waitForFinalTick: false,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (parseBooleanArg(options, arg)) {
      continue;
    }
    const nextIndex = parseValueArg(options, argv, index, arg);
    if (nextIndex !== index) {
      index = nextIndex;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  options.inputPath = resolveUserPath(options.inputPath);
  options.artifactDir = resolveUserPath(options.artifactDir);
  options.reportPath = options.reportPath
    ? path.resolve(process.cwd(), options.reportPath)
    : path.join(options.artifactDir, "profile-report.json");

  return options;
}

function parseBooleanArg(options, arg) {
  const booleanArgs = {
    "--compact-event-text": "compactEventText",
    "--disable-checkpoint-persistence": "disableCheckpointPersistence",
    "--heap-gc": "heapGC",
    "--heap-profile": "heapProfile",
    "--json": "json",
    "--reuse-build": "reuseBuild",
    "--source-maps": "sourceMaps",
    "--wait-for-final-tick": "waitForFinalTick",
  };
  const optionName = booleanArgs[arg];
  if (!optionName) {
    return false;
  }
  options[optionName] = true;
  return true;
}

function parseValueArg(options, argv, index, arg) {
  const stringArgs = {
    "--artifact-dir": "artifactDir",
    "--input": "inputPath",
    "--report": "reportPath",
  };
  const positiveIntegerArgs = {
    "--api-port": "apiPort",
    "--burst-size": "burstSize",
    "--heap-profile-sampling-interval": "heapProfileSamplingInterval",
    "--heap-profile-top-n": "heapProfileTopN",
    "--max-event-text-chars": "maxEventTextChars",
    "--max-events": "maxEvents",
  };
  const nonNegativeIntegerArgs = {
    "--delay-ms": "delayMs",
    "--settle-ms": "settleMs",
  };

  if (stringArgs[arg]) {
    options[stringArgs[arg]] = requiredValue(argv, index, arg);
    return index + 1;
  }
  if (positiveIntegerArgs[arg]) {
    options[positiveIntegerArgs[arg]] = parsePositiveInteger(
      requiredValue(argv, index, arg),
      arg,
    );
    return index + 1;
  }
  if (nonNegativeIntegerArgs[arg]) {
    options[nonNegativeIntegerArgs[arg]] = parseNonNegativeInteger(
      requiredValue(argv, index, arg),
      arg,
    );
    return index + 1;
  }
  return index;
}

function resolveUserPath(value) {
  if (value === "~") {
    return os.homedir();
  }
  if (value.startsWith("~/")) {
    return path.join(os.homedir(), value.slice(2));
  }
  return path.resolve(process.cwd(), value);
}

function requiredValue(argv, index, arg) {
  const value = argv[index + 1];
  if (!value) {
    throw new Error(`${arg} requires a value`);
  }
  return value;
}

function parsePositiveInteger(value, label) {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) {
    throw new Error(`${label} must be a positive integer, received ${value}`);
  }
  return parsed;
}

function parseNonNegativeInteger(value, label) {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(
      `${label} must be a non-negative integer, received ${value}`,
    );
  }
  return parsed;
}

function mb(bytes) {
  return Number((bytes / (1024 * 1024)).toFixed(1));
}

function bytesMB(bytes) {
  return Number((bytes / (1024 * 1024)).toFixed(2));
}

function durationMs(startedAt, endedAt = performance.now()) {
  return Number((endedAt - startedAt).toFixed(1));
}

async function readRecording(inputPath, maxEvents) {
  const text = await readFile(inputPath, "utf8").catch(async (error) => {
    if (inputPath.includes("/2026-6/")) {
      const correctedPath = inputPath.replace("/2026-6/", "/2026-06/");
      return await readFile(correctedPath, "utf8");
    }
    throw error;
  });
  const parsed = JSON.parse(text);
  const events = Array.isArray(parsed.events) ? parsed.events : [];
  const selectedEvents =
    maxEvents === null ? events : events.slice(0, maxEvents);

  if (selectedEvents.length === 0) {
    throw new Error(`No events found in recording: ${inputPath}`);
  }

  return {
    events: selectedEvents,
    recordedAt: parsed.recordedAt ?? null,
    schemaVersion: parsed.schemaVersion ?? null,
    sourceBytes: Buffer.byteLength(text),
    sourceEventCount: events.length,
  };
}

async function findAvailablePort() {
  const probe = http.createServer();
  await new Promise((resolve, reject) => {
    probe.once("error", reject);
    probe.listen(0, "127.0.0.1", resolve);
  });
  const address = probe.address();
  if (!address || typeof address === "string") {
    throw new Error("Expected TCP port probe to return an address.");
  }
  await new Promise((resolve, reject) => {
    probe.close((error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
  return address.port;
}

async function waitForURL(url, timeoutMs = 90_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch {
      // Keep polling until the preview server is ready.
    }
    await delay(250);
  }
  throw new Error(`Timed out waiting for ${url}.`);
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) {
    return;
  }
  child.kill("SIGTERM");
  await once(child, "exit");
}

async function startReusableBuildPreview(options) {
  await access(path.join(packageRoot, "dist", "index.html"));
  if (options.sourceMaps) {
    await access(path.join(packageRoot, "dist", "assets", "index.js.map"));
  }
  const apiPort = options.apiPort ?? 7437;
  const previewPort = await findAvailablePort();
  const apiOrigin = `http://127.0.0.1:${apiPort}`;
  const previewURL = `http://127.0.0.1:${previewPort}/dashboard/ui/`;
  const previewProcess = spawn(
    "bun",
    [
      "x",
      "vite",
      "preview",
      "--host",
      "127.0.0.1",
      "--port",
      String(previewPort),
      "--strictPort",
    ],
    {
      cwd: packageRoot,
      env: {
        ...process.env,
        AGENT_FACTORY_API_ORIGIN: apiOrigin,
        AGENT_FACTORY_PROFILE_SOURCEMAPS: options.sourceMaps ? "true" : "false",
        NODE_ENV: "production",
      },
      shell: false,
      stdio: "pipe",
    },
  );
  let stderr = "";
  previewProcess.stderr?.on("data", (chunk) => {
    stderr += chunk.toString();
  });
  previewProcess.once("exit", (code) => {
    if (code !== 0 && code !== null) {
      console.error(stderr.trim());
    }
  });
  await waitForURL(previewURL);
  return {
    apiOrigin,
    apiPort,
    previewPort,
    previewURL,
    stop: async () => {
      await stopProcess(previewProcess);
    },
  };
}

function summarizeEvents(events) {
  const typeCounts = {};
  let maxEventBytes = 0;
  let maxTick = 0;
  let totalEventBytes = 0;

  for (const event of events) {
    const type = event.type ?? "<missing>";
    const eventBytes = Buffer.byteLength(JSON.stringify(event));
    typeCounts[type] = (typeCounts[type] ?? 0) + 1;
    maxEventBytes = Math.max(maxEventBytes, eventBytes);
    maxTick = Math.max(maxTick, event.context?.tick ?? 0);
    totalEventBytes += eventBytes;
  }

  return {
    eventCount: events.length,
    maxEventMB: mb(maxEventBytes),
    maxTick,
    topTypes: Object.entries(typeCounts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 12)
      .map(([type, count]) => ({ count, type })),
    totalEventMB: mb(totalEventBytes),
  };
}

function findCurrentFactory(events) {
  for (const event of events) {
    const factory = event.payload?.factory;
    if (factory && typeof factory === "object") {
      return factory;
    }
  }
  return {
    factoryDirectory: "/profile/factory",
    name: "Recording Browser Profile",
    resources: [],
    sourceDirectory: "/profile/factory",
    workTypes: [],
    workers: [],
    workstations: [],
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: Local profile server keeps request handling explicit for debugging.
async function startProfileApiServer({
  apiPort,
  burstSize,
  currentFactory,
  delayMs,
  eventLines,
}) {
  let resolveReplayCompleted = () => {};
  const replayCompleted = new Promise((resolve) => {
    resolveReplayCompleted = resolve;
  });
  const requestedEventSessionIDs = [];
  const streamStats = {
    bytesWritten: 0,
    completed: false,
    eventsWritten: 0,
    notFoundRequests: [],
    openedAt: null,
    streamDurationMs: null,
  };

  const server = http.createServer(
    // biome-ignore lint/complexity/noExcessiveLinesPerFunction: Route stubs are intentionally colocated for profile-mode diagnostics.
    (request, response) => {
      if (request.method === "OPTIONS") {
        response.writeHead(204, {
          "Access-Control-Allow-Headers": "Content-Type",
          "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
          "Access-Control-Allow-Origin": "*",
        });
        response.end();
        return;
      }

      if (request.url === "/favicon.ico") {
        response.writeHead(204, {
          "Access-Control-Allow-Origin": "*",
        });
        response.end();
        return;
      }

      if (
        (request.url === "/status" ||
          request.url ===
            `/factory-sessions/${defaultFactorySessionID}/status`) &&
        request.method === "GET"
      ) {
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            currentSessionID: defaultFactorySessionID,
            factory_state: "RUNNING",
            managedRuntime: null,
            tick_count: 0,
          }),
        );
        return;
      }

      if (
        request.url?.startsWith("/provider-sessions/detail") &&
        request.method === "GET"
      ) {
        const requestURL = new URL(request.url, "http://127.0.0.1");
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            parse: {
              eventCount: 0,
              functionCalls: [],
              lineCount: 0,
              malformedLineCount: 0,
              parseErrors: [],
              reasoning: [],
              turns: [],
              unknownEventCount: 0,
              unknownEvents: [],
            },
            providerSession: {
              id: requestURL.searchParams.get("id") ?? "profile-session",
              kind: requestURL.searchParams.get("kind") ?? "session_id",
              provider: requestURL.searchParams.get("provider") ?? "codex",
            },
            source: {
              relativePath: "profile-mode.jsonl",
              sizeBytes: 0,
            },
            transcript: [],
          }),
        );
        return;
      }

      if (
        request.url?.match(promptTemplateContractPathPattern) &&
        request.method === "GET"
      ) {
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            availableVariables: [],
            inputCount: 0,
            unavailableAccessPatterns: [],
          }),
        );
        return;
      }

      if (
        request.url?.match(promptTemplateValidationPathPattern) &&
        request.method === "POST"
      ) {
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            diagnostics: [],
            valid: true,
          }),
        );
        return;
      }

      if (request.url === "/factory-sessions" && request.method === "GET") {
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            sessions: [
              {
                factoryDir:
                  currentFactory.factoryDirectory ?? "/profile/factory",
                folderPath:
                  currentFactory.sourceDirectory ?? "/profile/factory",
                id: defaultFactorySessionID,
                isDefault: true,
                project: currentFactory.name ?? "Recording Browser Profile",
                target: { kind: "default" },
              },
            ],
          }),
        );
        return;
      }

      if (
        request.url ===
          `/factory-sessions/${defaultFactorySessionID}/factory` &&
        request.method === "GET"
      ) {
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            ...currentFactory,
            version: {
              logical: "profile",
              physical: "2026-06-21T10:18:47.547969Z",
            },
          }),
        );
        return;
      }

      const requestURL = new URL(request.url ?? "/", "http://127.0.0.1");
      if (
        requestURL.pathname ===
          `/factory-sessions/${defaultFactorySessionID}/events` &&
        request.method === "GET"
      ) {
        const afterSequenceRaw = requestURL.searchParams.get("after_sequence");
        const afterSequence =
          afterSequenceRaw === null ? null : Number(afterSequenceRaw);
        const afterEventId = requestURL.searchParams.get("after_event_id");
        requestedEventSessionIDs.push(defaultFactorySessionID);
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
          const startedAt = performance.now();
          streamStats.openedAt = new Date().toISOString();
          for (let index = 0; index < eventLines.length; index += 1) {
            if (closed) {
              return;
            }
            if (
              !shouldReplayEventLine(
                eventLines[index],
                Number.isFinite(afterSequence) ? afterSequence : null,
                afterEventId,
              )
            ) {
              continue;
            }
            const block = `data: ${eventLines[index]}\n\n`;
            streamStats.bytesWritten += Buffer.byteLength(block);
            streamStats.eventsWritten += 1;
            response.write(block);
            if ((index + 1) % burstSize === 0 && delayMs > 0) {
              await delay(delayMs);
            }
          }
          if (!closed) {
            response.write(": replay-complete\n\n");
            streamStats.completed = true;
            streamStats.streamDurationMs = Number(
              (performance.now() - startedAt).toFixed(1),
            );
            resolveReplayCompleted();
          }
        })();
        return;
      }

      response.writeHead(404, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "text/plain; charset=utf-8",
      });
      streamStats.notFoundRequests.push({
        method: request.method,
        url: request.url,
      });
      response.end("not found");
    },
  );

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(apiPort, "127.0.0.1", resolve);
  });

  return {
    replayCompleted,
    requestedEventSessionIDs,
    stats: streamStats,
    stop: async () => {
      server.closeAllConnections?.();
      await new Promise((resolve, reject) => {
        server.close((error) => {
          if (error) {
            reject(error);
            return;
          }
          resolve();
        });
      });
    },
  };
}

async function installPerformanceProbe(page) {
  await page.addInitScript(() => {
    window.__youRecordingProfile = {
      eventLoopLagSamples: [],
      longTasks: [],
      marks: {
        initializedAt: performance.now(),
      },
    };

    const profile = window.__youRecordingProfile;
    let expectedAt = performance.now() + 50;
    window.setInterval(() => {
      const now = performance.now();
      profile.eventLoopLagSamples.push(Math.max(0, now - expectedAt));
      expectedAt = now + 50;
    }, 50);

    try {
      const observer = new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
          profile.longTasks.push({
            duration: entry.duration,
            name: entry.name,
            startTime: entry.startTime,
          });
        }
      });
      observer.observe({ entryTypes: ["longtask"] });
      profile.longTaskObserver = "enabled";
    } catch (error) {
      profile.longTaskObserver = `unavailable: ${error.message}`;
    }

    window.addEventListener("load", () => {
      profile.marks.windowLoadedAt = performance.now();
    });
  });
}

function percentile(values, percentileValue) {
  if (values.length === 0) {
    return 0;
  }
  const sorted = [...values].sort((a, b) => a - b);
  const index = Math.min(
    sorted.length - 1,
    Math.floor((percentileValue / 100) * sorted.length),
  );
  return Number(sorted[index].toFixed(1));
}

function profileURL(previewURL, options) {
  const url = new URL(previewURL);
  if (options.compactEventText) {
    url.searchParams.set("afCompactEventText", "1");
  }
  if (options.maxEventTextChars !== null) {
    url.searchParams.set(
      "afMaxEventTextChars",
      String(options.maxEventTextChars),
    );
  }
  if (options.disableCheckpointPersistence) {
    url.searchParams.set("afDisableTimelineCheckpoint", "1");
  }
  return url.toString();
}

function shouldReplayEventLine(line, afterSequence, afterEventId) {
  if (afterSequence === null && afterEventId === null) {
    return true;
  }
  try {
    const event = JSON.parse(line);
    if (afterEventId !== null && event.id === afterEventId) {
      return false;
    }
    const sequence = event.context?.sessionSequence ?? event.context?.sequence;
    return typeof sequence === "number" && afterSequence !== null
      ? sequence > afterSequence
      : true;
  } catch {
    return true;
  }
}

async function collectBrowserMetrics(page) {
  return await page.evaluate(() => {
    const profile = window.__youRecordingProfile;
    const longTasks = profile?.longTasks ?? [];
    const lagSamples = profile?.eventLoopLagSamples ?? [];
    const memory = performance.memory
      ? {
          jsHeapSizeLimitMB: Math.round(
            performance.memory.jsHeapSizeLimit / 1024 / 1024,
          ),
          totalJSHeapSizeMB: Math.round(
            performance.memory.totalJSHeapSize / 1024 / 1024,
          ),
          usedJSHeapSizeMB: Math.round(
            performance.memory.usedJSHeapSize / 1024 / 1024,
          ),
        }
      : null;

    return {
      documentReadyState: document.readyState,
      domNodeCount: document.querySelectorAll("*").length,
      lagSamples,
      longTaskCount: longTasks.length,
      longTaskObserver: profile?.longTaskObserver ?? "missing",
      longTasks: longTasks
        .slice()
        .sort((a, b) => b.duration - a.duration)
        .slice(0, 20)
        .map((entry) => ({
          durationMs: Number(entry.duration.toFixed(1)),
          name: entry.name,
          startTimeMs: Number(entry.startTime.toFixed(1)),
        })),
      marks: profile?.marks ?? {},
      memory,
      navigation: performance.getEntriesByType("navigation").map((entry) => ({
        domContentLoadedMs: Number(entry.domContentLoadedEventEnd.toFixed(1)),
        loadEventEndMs: Number(entry.loadEventEnd.toFixed(1)),
        responseEndMs: Number(entry.responseEnd.toFixed(1)),
      }))[0],
    };
  });
}

async function startHeapProfiler(page, options) {
  if (!options.heapProfile) {
    if (!options.heapGC) {
      return null;
    }
    const cdpSession = await page.context().newCDPSession(page);
    await cdpSession.send("Runtime.enable");
    await cdpSession.send("Performance.enable");
    await cdpSession.send("HeapProfiler.enable");
    return cdpSession;
  }

  const cdpSession = await page.context().newCDPSession(page);
  await cdpSession.send("Runtime.enable");
  await cdpSession.send("Performance.enable");
  await cdpSession.send("HeapProfiler.enable");
  await cdpSession.send("HeapProfiler.startSampling", {
    samplingInterval: options.heapProfileSamplingInterval,
  });

  return cdpSession;
}

async function collectHeapProfile(cdpSession, options) {
  if (!cdpSession) {
    return null;
  }

  const beforeGC = await cdpSession.send("Runtime.getHeapUsage");
  const beforePerformance = await cdpPerformanceHeap(cdpSession);
  const samplingProfile = options.heapProfile
    ? await cdpSession.send("HeapProfiler.stopSampling")
    : null;
  for (let index = 0; index < 3; index += 1) {
    await cdpSession.send("HeapProfiler.collectGarbage");
    await delay(100);
  }
  const afterGC = await cdpSession.send("Runtime.getHeapUsage");
  const afterPerformance = await cdpPerformanceHeap(cdpSession);

  return {
    afterGC: heapUsageSummary(afterGC),
    afterPerformance,
    beforeGC: heapUsageSummary(beforeGC),
    beforePerformance,
    sampling: samplingProfile
      ? await summarizeSamplingHeapProfile(
          samplingProfile.profile,
          options.heapProfileTopN,
          options,
        )
      : null,
  };
}

async function cdpPerformanceHeap(cdpSession) {
  const metricsResponse = await cdpSession.send("Performance.getMetrics");
  const metrics = Object.fromEntries(
    metricsResponse.metrics.map((metric) => [metric.name, metric.value]),
  );
  return {
    totalSizeMB: bytesMB(metrics.JSHeapTotalSize ?? 0),
    usedSizeMB: bytesMB(metrics.JSHeapUsedSize ?? 0),
  };
}

function heapUsageSummary(usage) {
  return {
    totalSizeMB: bytesMB(usage.totalSize),
    usedSizeMB: bytesMB(usage.usedSize),
  };
}

async function summarizeSamplingHeapProfile(profile, topN, options) {
  const nodes = [];
  walkSamplingHeapNode(profile.head, nodes);
  const sourceMapConsumer = options.sourceMaps
    ? await loadProfileSourceMap()
    : null;
  const totalSelfSize = nodes.reduce((sum, node) => sum + node.selfSize, 0);
  const topSelfSizeNodes = nodes
    .filter((node) => node.selfSize > 0)
    .sort((left, right) => right.selfSize - left.selfSize)
    .slice(0, topN)
    .map((node) => samplingNodeSummary(node, totalSelfSize, sourceMapConsumer));

  sourceMapConsumer?.destroy?.();

  return {
    nodeCount: nodes.length,
    topSelfSizeNodes,
    totalSelfSizeMB: bytesMB(totalSelfSize),
  };
}

async function loadProfileSourceMap() {
  const sourceMapPath = path.join(
    packageRoot,
    "dist",
    "assets",
    "index.js.map",
  );
  try {
    const rawSourceMap = await readFile(sourceMapPath, "utf8");
    return await new SourceMapConsumer(JSON.parse(rawSourceMap));
  } catch {
    return null;
  }
}

function samplingNodeSummary(node, totalSelfSize, sourceMapConsumer) {
  const original =
    sourceMapConsumer && node.lineNumber !== undefined && node.lineNumber >= 0
      ? sourceMapConsumer.originalPositionFor({
          column: node.columnNumber ?? 0,
          line: node.lineNumber + 1,
        })
      : null;
  return {
    columnNumber: node.columnNumber,
    functionName: node.functionName,
    lineNumber: node.lineNumber,
    originalColumn: original?.column ?? undefined,
    originalLine: original?.line ?? undefined,
    originalName: original?.name ?? undefined,
    originalSource: original?.source ?? undefined,
    selfSizeMB: bytesMB(node.selfSize),
    selfSizePercent:
      totalSelfSize > 0
        ? Number(((node.selfSize / totalSelfSize) * 100).toFixed(1))
        : 0,
    scriptURL: node.scriptURL,
  };
}

function walkSamplingHeapNode(node, nodes) {
  const callFrame = node.callFrame ?? {};
  nodes.push({
    functionName: callFrame.functionName || "(anonymous)",
    columnNumber: callFrame.columnNumber,
    lineNumber: callFrame.lineNumber,
    scriptURL: callFrame.url || "",
    selfSize: node.selfSize ?? 0,
  });
  for (const child of node.children ?? []) {
    walkSamplingHeapNode(child, nodes);
  }
}

function buildFindings({ browserMetrics, eventSummary, streamStats, timing }) {
  const lagSamples = browserMetrics.lagSamples ?? [];
  const maxLag = percentile(lagSamples, 100);
  const p95Lag = percentile(lagSamples, 95);
  const findings = [
    `Total profile wall time was ${timing.totalBeforeReportWriteMs} ms before report write.`,
    `Dashboard exercise time was ${timing.dashboardExerciseMs} ms from navigation start through metric collection.`,
    `Replayed ${eventSummary.eventCount} events (${eventSummary.totalEventMB} MB JSON) through the browser event stream.`,
    `The stream wrote ${streamStats.eventsWritten} events in ${streamStats.streamDurationMs ?? "unknown"} ms.`,
    `Chromium observed ${browserMetrics.longTaskCount} long tasks; worst task ${browserMetrics.longTasks[0]?.durationMs ?? 0} ms.`,
    `Event-loop lag p95 ${p95Lag} ms, max ${maxLag} ms.`,
  ];

  if (browserMetrics.memory) {
    findings.push(
      `Browser JS heap after replay: ${browserMetrics.memory.usedJSHeapSizeMB} MB used of ${browserMetrics.memory.totalJSHeapSizeMB} MB committed.`,
    );
  }
  if (browserMetrics.heapProfile) {
    findings.push(
      `Forced-GC JS heap after replay: ${browserMetrics.heapProfile.afterGC.usedSizeMB} MB used of ${browserMetrics.heapProfile.afterGC.totalSizeMB} MB total.`,
    );
    findings.push(
      `CDP Performance JS heap after forced GC: ${browserMetrics.heapProfile.afterPerformance.usedSizeMB} MB used of ${browserMetrics.heapProfile.afterPerformance.totalSizeMB} MB total.`,
    );
    if (browserMetrics.heapProfile.sampling) {
      findings.push(
        `Sampled heap allocations total ${browserMetrics.heapProfile.sampling.totalSelfSizeMB} MB; top sampled frame ${browserMetrics.heapProfile.sampling.topSelfSizeNodes[0]?.selfSizeMB ?? 0} MB in ${browserMetrics.heapProfile.sampling.topSelfSizeNodes[0]?.functionName ?? "unknown"}.`,
      );
    }
  }

  return findings;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: The CLI flow stays linear so profile phase timings remain easy to audit.
async function main() {
  const totalStartedAt = performance.now();
  const options = parseArgs(process.argv.slice(2));
  const readStartedAt = performance.now();
  const recording = await readRecording(options.inputPath, options.maxEvents);
  const recordingReadMs = durationMs(readStartedAt);
  const prepareStartedAt = performance.now();
  const eventSummary = summarizeEvents(recording.events);
  const currentFactory = findCurrentFactory(recording.events);
  const eventLines = recording.events.map((event) => JSON.stringify(event));
  await mkdir(options.artifactDir, { recursive: true });
  const prepareMs = durationMs(prepareStartedAt);

  const previousArtifactDir = process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;
  const previousSourceMapFlag = process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS;
  process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR = path.relative(
    packageRoot,
    options.artifactDir,
  );
  process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS = options.sourceMaps
    ? "true"
    : "false";

  let preview = null;
  let apiServer = null;
  let browserPage = null;
  let heapProfiler = null;

  try {
    const previewStartedAt = performance.now();
    preview = options.reuseBuild
      ? await startReusableBuildPreview(options)
      : await startBrowserPreview();
    const previewStartMs = durationMs(previewStartedAt);
    const apiStartedAt = performance.now();
    apiServer = await startProfileApiServer({
      apiPort: preview.apiPort,
      burstSize: options.burstSize,
      currentFactory,
      delayMs: options.delayMs,
      eventLines,
    });
    const apiStartMs = durationMs(apiStartedAt);
    const browserStartedAt = performance.now();
    browserPage = await openBrowserPage({
      artifactLabel: "recording-profile",
    });
    await installPerformanceProbe(browserPage.page);
    heapProfiler = await startHeapProfiler(browserPage.page, options);
    const browserOpenMs = durationMs(browserStartedAt);

    const navigationStartedAt = performance.now();
    await browserPage.page.goto(profileURL(preview.previewURL, options), {
      waitUntil: "domcontentloaded",
      timeout: 60_000,
    });
    const navigationMs = durationMs(navigationStartedAt);
    const boardReadyStartedAt = performance.now();
    await browserPage.page
      .getByRole("region", { name: "you-agent-factory bento board" })
      .waitFor({ timeout: uiInteractionTimeoutMs });
    const boardReadyMs = durationMs(boardReadyStartedAt);
    const replayWaitStartedAt = performance.now();
    await apiServer.replayCompleted;
    const replayWaitMs = durationMs(replayWaitStartedAt);
    const settleStartedAt = performance.now();
    await delay(options.settleMs);
    const settleMs = durationMs(settleStartedAt);

    if (options.waitForFinalTick) {
      const finalTickStartedAt = performance.now();
      await browserPage.page
        .getByText(`${eventSummary.maxTick}/${eventSummary.maxTick}`)
        .waitFor({ timeout: 60_000 });
      apiServer.stats.finalTickVisibleMs = durationMs(finalTickStartedAt);
    }

    const collectStartedAt = performance.now();
    const browserMetrics = await collectBrowserMetrics(browserPage.page);
    browserMetrics.heapProfile = await collectHeapProfile(
      heapProfiler,
      options,
    );
    const collectMetricsMs = durationMs(collectStartedAt);
    const totalBeforeReportWriteMs = durationMs(totalStartedAt);
    const timing = {
      apiStartMs,
      boardReadyMs,
      browserOpenMs,
      collectMetricsMs,
      dashboardExerciseMs: durationMs(navigationStartedAt),
      navigationMs,
      prepareRecordingMs: prepareMs,
      previewStartMs,
      recordingReadMs,
      replayWaitMs,
      settleMs,
      totalBeforeReportWriteMs,
    };
    const report = {
      browser: browserMetrics,
      consoleErrors: browserPage.consoleErrors,
      durationMs: totalBeforeReportWriteMs,
      eventSummary,
      findings: buildFindings({
        browserMetrics,
        eventSummary,
        streamStats: apiServer.stats,
        timing,
      }),
      pageErrors: browserPage.pageErrors,
      recording: {
        inputPath: options.inputPath,
        apiPort: preview.apiPort,
        previewPort: preview.previewPort,
        recordedAt: recording.recordedAt,
        reuseBuild: options.reuseBuild,
        schemaVersion: recording.schemaVersion,
        sourceEventCount: recording.sourceEventCount,
        sourceMB: mb(recording.sourceBytes),
      },
      dashboardOptions: {
        checkpointPersistenceDisabled: options.disableCheckpointPersistence,
        sourceMaps: options.sourceMaps,
      },
      heapProfile:
        options.heapGC || options.heapProfile
          ? {
              gcOnly: options.heapGC && !options.heapProfile,
              samplingInterval: options.heapProfileSamplingInterval,
              topN: options.heapProfileTopN,
            }
          : undefined,
      requestedEventSessionIDs: apiServer.requestedEventSessionIDs,
      stream: {
        ...apiServer.stats,
        bytesWrittenMB: mb(apiServer.stats.bytesWritten),
      },
      timing,
    };

    const reportWriteStartedAt = performance.now();
    await writeFile(
      options.reportPath,
      JSON.stringify(report, null, 2),
      "utf8",
    );
    report.timing.reportWriteMs = durationMs(reportWriteStartedAt);
    report.timing.totalMs = durationMs(totalStartedAt);
    report.durationMs = report.timing.totalMs;
    await writeFile(
      options.reportPath,
      JSON.stringify(report, null, 2),
      "utf8",
    );

    if (options.json) {
      process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
    } else {
      console.log("recording browser profile");
      console.log(`input: ${options.inputPath}`);
      console.log(`report: ${options.reportPath}`);
      console.log("");
      for (const finding of report.findings) {
        console.log(`- ${finding}`);
      }
    }
  } finally {
    if (browserPage) {
      await browserPage.close();
    }
    if (apiServer) {
      await apiServer.stop();
    }
    if (preview) {
      await preview.stop();
    }
    if (typeof previousArtifactDir === "undefined") {
      delete process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;
    } else {
      process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR = previousArtifactDir;
    }
    if (typeof previousSourceMapFlag === "undefined") {
      delete process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS;
    } else {
      process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS = previousSourceMapFlag;
    }
  }
}

main().catch((error) => {
  console.error(error.stack ?? error.message);
  process.exitCode = 1;
});
