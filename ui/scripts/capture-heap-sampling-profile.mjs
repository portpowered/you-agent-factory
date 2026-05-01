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

function parsePositiveInteger(value, flagName) {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) {
    throw new Error(`${flagName} must be a positive integer, received ${value ?? "<missing>"}`);
  }
  return parsed;
}

function parseArgs(argv) {
  const options = {
    durationMs: 20_000,
    outDir: defaultOutDir,
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
    if (arg === "--duration-ms") {
      options.durationMs = parsePositiveInteger(argv[index + 1], "--duration-ms");
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  return options;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function isoStamp() {
  return new Date().toISOString().replaceAll(":", "-");
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  await fs.promises.mkdir(options.outDir, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();
  const cdpSession = await context.newCDPSession(page);

  await cdpSession.send("HeapProfiler.enable");
  await page.goto(options.url, {
    timeout: 60_000,
    waitUntil: "domcontentloaded",
  });

  await cdpSession.send("HeapProfiler.startSampling", {
    samplingInterval: 32 * 1024,
  });
  await sleep(options.durationMs);
  const samplingProfile = await cdpSession.send("HeapProfiler.stopSampling");

  const debugSummary = await page.evaluate(() => {
    return globalThis.window?.__agentFactoryTimelineDebug__?.summarize?.() ?? null;
  });

  const stamp = isoStamp();
  const profilePath = path.join(options.outDir, `heap-sampling-${stamp}.heapprofile.json`);
  const summaryPath = path.join(options.outDir, `heap-sampling-${stamp}.summary.json`);

  await fs.promises.writeFile(profilePath, JSON.stringify(samplingProfile, null, 2));
  await fs.promises.writeFile(
    summaryPath,
    JSON.stringify(
      {
        capturedAt: new Date().toISOString(),
        debugSummary,
        durationMs: options.durationMs,
        profilePath,
        url: options.url,
      },
      null,
      2,
    ),
  );

  await browser.close();

  console.log(`heap sampling summary: ${summaryPath}`);
  console.log(`heap sampling profile: ${profilePath}`);
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack ?? error.message : String(error));
  process.exitCode = 1;
});
