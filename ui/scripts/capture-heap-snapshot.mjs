import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const repoRoot = path.resolve(packageRoot, "..", "..", "..");
const defaultOutDir = path.join(repoRoot, "factory", "logs", "browser-heap");
const defaultURL = "http://127.0.0.1:4173/dashboard/ui/";
const defaultSnapshotDelayMs = 20_000;
const defaultSampleIntervalMs = 5_000;
const defaultTimeoutMs = 90_000;

function parseBooleanFlag(value, fallback) {
  if (value === undefined) {
    return fallback;
  }
  if (value === "true" || value === "1") {
    return true;
  }
  if (value === "false" || value === "0") {
    return false;
  }
  throw new Error(`Expected boolean flag, received ${value}`);
}

function parsePositiveInteger(value, flagName) {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) {
    throw new Error(`${flagName} must be a positive integer, received ${value ?? "<missing>"}`);
  }
  return parsed;
}

function parseArgs(argv) {
  const options = {
    headless: true,
    outDir: defaultOutDir,
    sampleIntervalMs: defaultSampleIntervalMs,
    snapshotDelayMs: defaultSnapshotDelayMs,
    timeoutMs: defaultTimeoutMs,
    url: defaultURL,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--url") {
      options.url = argv[index + 1] ?? "";
      index += 1;
      continue;
    }
    if (arg === "--out-dir") {
      options.outDir = path.resolve(process.cwd(), argv[index + 1] ?? "");
      index += 1;
      continue;
    }
    if (arg === "--snapshot-delay-ms") {
      options.snapshotDelayMs = parsePositiveInteger(argv[index + 1], "--snapshot-delay-ms");
      index += 1;
      continue;
    }
    if (arg === "--sample-interval-ms") {
      options.sampleIntervalMs = parsePositiveInteger(argv[index + 1], "--sample-interval-ms");
      index += 1;
      continue;
    }
    if (arg === "--timeout-ms") {
      options.timeoutMs = parsePositiveInteger(argv[index + 1], "--timeout-ms");
      index += 1;
      continue;
    }
    if (arg === "--headless") {
      options.headless = parseBooleanFlag(argv[index + 1], true);
      index += 1;
      continue;
    }
    if (arg === "--help") {
      printHelp();
      process.exit(0);
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  return options;
}

function printHelp() {
  console.log("capture-heap-snapshot.mjs");
  console.log("");
  console.log("Usage:");
  console.log("  node scripts/capture-heap-snapshot.mjs [options]");
  console.log("");
  console.log("Options:");
  console.log(`  --url <value>                 Page URL to open. Default: ${defaultURL}`);
  console.log(`  --out-dir <path>              Snapshot output directory. Default: ${defaultOutDir}`);
  console.log(`  --snapshot-delay-ms <value>   Delay before taking the heap snapshot. Default: ${defaultSnapshotDelayMs}`);
  console.log(`  --sample-interval-ms <value>  Interval between memory samples. Default: ${defaultSampleIntervalMs}`);
  console.log(`  --timeout-ms <value>          Overall timeout. Default: ${defaultTimeoutMs}`);
  console.log("  --headless <true|false>       Launch Chromium headless. Default: true");
}

function sleep(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

function isoStamp() {
  return new Date().toISOString().replaceAll(":", "-");
}

async function ensureOutputDirectory(outDir) {
  await fs.promises.mkdir(outDir, { recursive: true });
}

async function capturePageSample(page, cdpSession) {
  const [performanceMetrics, pageMemory] = await Promise.all([
    cdpSession.send("Performance.getMetrics"),
    page.evaluate(() => {
      const performanceMemory = globalThis.performance?.memory;
      const debugGlobal = globalThis.window?.__agentFactoryTimelineDebug__;
      return {
        debugSummary: typeof debugGlobal?.summarize === "function" ? debugGlobal.summarize() : null,
        jsHeapSizeLimit:
          typeof performanceMemory?.jsHeapSizeLimit === "number"
            ? performanceMemory.jsHeapSizeLimit
            : null,
        totalJSHeapSize:
          typeof performanceMemory?.totalJSHeapSize === "number"
            ? performanceMemory.totalJSHeapSize
            : null,
        usedJSHeapSize:
          typeof performanceMemory?.usedJSHeapSize === "number"
            ? performanceMemory.usedJSHeapSize
            : null,
      };
    }),
  ]);

  const metrics = Object.fromEntries(
    (performanceMetrics.metrics ?? []).map((metric) => [metric.name, metric.value]),
  );

  return {
    capturedAt: new Date().toISOString(),
    debugSummary: pageMemory.debugSummary,
    domNodes: metrics.Nodes ?? null,
    jsEventListeners: metrics.JSEventListeners ?? null,
    jsHeapSizeLimit: pageMemory.jsHeapSizeLimit,
    layoutCount: metrics.LayoutCount ?? null,
    processPrivateMemory: metrics.ProcessPrivateMemory ?? null,
    processResidentMemory: metrics.ProcessResidentMemory ?? null,
    recalcStyleCount: metrics.RecalcStyleCount ?? null,
    totalJSHeapSize: pageMemory.totalJSHeapSize,
    usedJSHeapSize: pageMemory.usedJSHeapSize,
  };
}

async function writeJson(filePath, value) {
  await fs.promises.writeFile(filePath, JSON.stringify(value, null, 2));
}

async function takeHeapSnapshot(cdpSession, snapshotPath) {
  const output = fs.createWriteStream(snapshotPath, { encoding: "utf8" });
  let chunkCount = 0;
  let totalBytes = 0;
  const chunkListener = ({ chunk }) => {
    chunkCount += 1;
    totalBytes += Buffer.byteLength(chunk);
    output.write(chunk);
    if (chunkCount % 250 === 0) {
      console.log(
        `heap snapshot progress: ${chunkCount} chunks, ${(totalBytes / (1024 * 1024)).toFixed(1)} MB`,
      );
    }
  };

  cdpSession.on("HeapProfiler.addHeapSnapshotChunk", chunkListener);
  try {
    await cdpSession.send("HeapProfiler.takeHeapSnapshot", {
      reportProgress: false,
    });
  } finally {
    cdpSession.off("HeapProfiler.addHeapSnapshotChunk", chunkListener);
    await new Promise((resolve, reject) => {
      output.end((error) => {
        if (error) {
          reject(error);
          return;
        }
        resolve();
      });
    });
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  await ensureOutputDirectory(options.outDir);

  const runStamp = isoStamp();
  const summaryPath = path.join(options.outDir, `heap-capture-${runStamp}.summary.json`);
  const snapshotPath = path.join(options.outDir, `heap-capture-${runStamp}.heapsnapshot`);

  const browser = await chromium.launch({
    headless: options.headless,
  });

  const context = await browser.newContext();
  const page = await context.newPage();
  const cdpSession = await context.newCDPSession(page);
  const samples = [];
  let crashed = false;
  let closed = false;

  page.on("crash", () => {
    crashed = true;
  });
  page.on("close", () => {
    closed = true;
  });

  await cdpSession.send("Performance.enable");
  await cdpSession.send("HeapProfiler.enable");

  const startedAt = Date.now();
  await page.goto(options.url, {
    timeout: options.timeoutMs,
    waitUntil: "domcontentloaded",
  });

  while (Date.now() - startedAt < options.snapshotDelayMs) {
    if (crashed || closed) {
      break;
    }
    samples.push(await capturePageSample(page, cdpSession));
    await sleep(options.sampleIntervalMs);
  }

  if (!crashed && !closed) {
    samples.push(await capturePageSample(page, cdpSession));
    await takeHeapSnapshot(cdpSession, snapshotPath);
  }

  const summary = {
    closed,
    crashed,
    endedAt: new Date().toISOString(),
    sampleCount: samples.length,
    samples,
    snapshotCaptured: !crashed && !closed,
    snapshotPath: !crashed && !closed ? snapshotPath : null,
    startedAt: new Date(startedAt).toISOString(),
    url: options.url,
  };

  await writeJson(summaryPath, summary);
  await browser.close();

  console.log(`heap capture summary: ${summaryPath}`);
  if (summary.snapshotCaptured) {
    console.log(`heap snapshot: ${snapshotPath}`);
  } else {
    console.log("heap snapshot was not captured before the page closed or crashed");
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack ?? error.message : String(error));
  process.exitCode = 1;
});
