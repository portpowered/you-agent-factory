import { spawnSync } from "node:child_process";
import { performance } from "node:perf_hooks";

import {
  defaultCapturedStdoutMaxBuffer,
  formatElapsedMs,
  logSlowFileSummary,
} from "./ui-test-cost-report.mjs";

export const phaseLogPrefix = "[ui-browser-integration]";
export const browserIntegrationPhaseName = "Browser integration Vitest pass";

const browserIntegrationWorkerArgs = [
  "--no-file-parallelism",
  "--maxWorkers",
  "1",
];

export function buildBrowserIntegrationVitestArgs() {
  return ["run", "--dir", "integration", ...browserIntegrationWorkerArgs];
}

export function buildFocusedBrowserIntegrationVitestArgs(files) {
  if (!Array.isArray(files) || files.length === 0) {
    throw new Error("buildFocusedBrowserIntegrationVitestArgs requires files.");
  }

  return ["run", ...files, ...browserIntegrationWorkerArgs];
}

export function formatPhaseElapsed(phaseName, elapsedMs) {
  return `${phaseLogPrefix} ${phaseName} elapsed: ${formatElapsedMs(elapsedMs)}`;
}

function runVitestIntegrationPass({
  vitestArgs,
  phaseName,
  slowFileSummaryTitle,
  spawn = spawnSync,
}) {
  const startedAt = performance.now();
  const result = spawn("vitest", vitestArgs, {
    shell: process.platform === "win32",
    stdio: ["inherit", "pipe", "inherit"],
    encoding: "utf8",
    maxBuffer: defaultCapturedStdoutMaxBuffer,
  });
  const elapsedMs = performance.now() - startedAt;

  if (result.stdout) {
    process.stdout.write(result.stdout);
  }

  console.log(formatPhaseElapsed(phaseName, elapsedMs));

  if (result.error) {
    throw result.error;
  }

  logSlowFileSummary(result.stdout ?? "", {
    logPrefix: phaseLogPrefix,
    summaryTitle: slowFileSummaryTitle,
  });

  const status = result.status ?? 1;
  if (status !== 0) {
    process.exit(status);
  }
}

export function runBrowserIntegration(options = {}) {
  runVitestIntegrationPass({
    vitestArgs: buildBrowserIntegrationVitestArgs(),
    phaseName: browserIntegrationPhaseName,
    slowFileSummaryTitle: "Browser integration slowest test files",
    spawn: options.spawn,
  });
}

export function runFocusedBrowserIntegration(files, options = {}) {
  runVitestIntegrationPass({
    vitestArgs: buildFocusedBrowserIntegrationVitestArgs(files),
    phaseName:
      options.phaseName ??
      `Focused browser integration Vitest pass (${files.length} files)`,
    slowFileSummaryTitle: "Focused browser integration slowest test files",
    spawn: options.spawn,
  });
}
