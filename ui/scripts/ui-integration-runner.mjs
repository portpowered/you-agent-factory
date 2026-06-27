import { spawnSync } from "node:child_process";
import { performance } from "node:perf_hooks";

import {
  defaultCapturedStdoutMaxBuffer,
  formatElapsedMs,
  logSlowFileSummary,
} from "./ui-test-cost-report.mjs";

export const phaseLogPrefix = "[ui-browser-integration]";
export const browserIntegrationPhaseName = "Browser integration Vitest pass";

export function buildBrowserIntegrationVitestArgs() {
  return [
    "run",
    "--dir",
    "integration",
    "--no-file-parallelism",
    "--maxWorkers",
    "1",
  ];
}

export function formatPhaseElapsed(phaseName, elapsedMs) {
  return `${phaseLogPrefix} ${phaseName} elapsed: ${formatElapsedMs(elapsedMs)}`;
}

export function runBrowserIntegration(options = {}) {
  const spawn = options.spawn ?? spawnSync;
  const startedAt = performance.now();
  const result = spawn("vitest", buildBrowserIntegrationVitestArgs(), {
    shell: process.platform === "win32",
    stdio: ["inherit", "pipe", "inherit"],
    encoding: "utf8",
    maxBuffer: defaultCapturedStdoutMaxBuffer,
  });
  const elapsedMs = performance.now() - startedAt;

  if (result.stdout) {
    process.stdout.write(result.stdout);
  }

  console.log(formatPhaseElapsed(browserIntegrationPhaseName, elapsedMs));

  if (result.error) {
    throw result.error;
  }

  logSlowFileSummary(result.stdout ?? "", {
    logPrefix: phaseLogPrefix,
    summaryTitle: "Browser integration slowest test files",
  });

  const status = result.status ?? 1;
  if (status !== 0) {
    process.exit(status);
  }
}
