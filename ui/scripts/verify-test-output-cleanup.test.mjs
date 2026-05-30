// @vitest-environment node

import { spawn, spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test } from "bun:test";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const blockedWarningFragments = [
  "The width(-1) and height(-1) of chart should be greater than 0",
  "not wrapped in act",
];
const warningCleanupLanes = [
  {
    file: "src/features/work-outcome/components/work-chart-warning-regression.test.tsx",
    timeoutMs: 45_000,
  },
  {
    file: "src/features/header/components/tick-slider-control.test.tsx",
    timeoutMs: 45_000,
  },
];

function bunCommand() {
  return process.platform === "win32" ? "bun.exe" : "bun";
}

function npmCommand() {
  return process.platform === "win32" ? "npm.cmd" : "npm";
}

function hasBun() {
  const result = spawnSync(bunCommand(), ["--version"], {
    shell: false,
    stdio: "ignore",
  });

  return result.status === 0;
}

function resolveRuntimeCommand(testFile) {
  if (hasBun()) {
    return {
      args: ["x", "vitest", "run", testFile],
      command: bunCommand(),
    };
  }

  return {
    args: ["exec", "--", "vitest", "run", testFile],
    command: npmCommand(),
  };
}

function createChildEnv() {
  const env = {
    ...process.env,
  };

  delete env.NODE_ENV;
  for (const key of Object.keys(env)) {
    if (key === "VITEST" || key.startsWith("VITEST_")) {
      delete env[key];
    }
  }

  return env;
}

async function runWarningCleanupLane(testFile) {
  const runtime = resolveRuntimeCommand(testFile);
  const child = spawn(runtime.command, runtime.args, {
    cwd: packageRoot,
    env: createChildEnv(),
    shell: false,
    stdio: "pipe",
  });

  let stdout = "";
  let stderr = "";

  child.stdout?.on("data", (chunk) => {
    stdout += chunk.toString();
  });
  child.stderr?.on("data", (chunk) => {
    stderr += chunk.toString();
  });

  const exitCode = await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("close", resolve);
  });

  return {
    exitCode,
    stderr,
    stdout,
  };
}

for (const lane of warningCleanupLanes) {
  test(
    `warning-prone UI regression lane stays quiet for ${lane.file}`,
    async () => {
      const result = await runWarningCleanupLane(lane.file);
      const combinedOutput = `${result.stdout}\n${result.stderr}`;

      expect(result.exitCode).toBe(0);
      for (const blockedWarningFragment of blockedWarningFragments) {
        expect(combinedOutput).not.toContain(blockedWarningFragment);
      }
    },
    lane.timeoutMs,
  );
}
