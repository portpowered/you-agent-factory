import { spawnSync } from "node:child_process";
import { performance } from "node:perf_hooks";
import {
  mockedBackendBrowserIntegrationFiles,
  mockedBackendBrowserIntegrationPhaseName,
} from "./ui-integration-targets.mjs";
import {
  defaultCapturedStdoutMaxBuffer,
  formatElapsedMs,
  logSlowFileSummary,
} from "./ui-test-cost-report.mjs";

export const phaseLogPrefix = "[ui-browser-integration]";
export const browserIntegrationPhaseName =
  mockedBackendBrowserIntegrationPhaseName;

const focusedBrowserIntegrationWorkerArgs = [
  "--no-file-parallelism",
  "--maxWorkers",
  "1",
];

export function browserIntegrationMaxWorkers(env = process.env) {
  const configured = Number(env.UI_BROWSER_INTEGRATION_MAX_WORKERS ?? "2");
  if (!Number.isInteger(configured) || configured < 1) {
    throw new Error(
      `UI_BROWSER_INTEGRATION_MAX_WORKERS must be a positive integer, got "${env.UI_BROWSER_INTEGRATION_MAX_WORKERS}".`,
    );
  }
  return configured;
}

export function buildBrowserIntegrationVitestArgs(env = process.env) {
  return [
    "run",
    ...mockedBackendBrowserIntegrationFiles,
    "--fileParallelism",
    "--maxWorkers",
    String(browserIntegrationMaxWorkers(env)),
    "--reporter=verbose",
  ];
}

export function buildFocusedBrowserIntegrationVitestArgs(files) {
  if (!Array.isArray(files) || files.length === 0) {
    throw new Error("buildFocusedBrowserIntegrationVitestArgs requires files.");
  }

  return ["run", ...files, ...focusedBrowserIntegrationWorkerArgs];
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
    env: {
      ...process.env,
      AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION: "true",
    },
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
