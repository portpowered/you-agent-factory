// @vitest-environment node

import { spawn, spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test } from "vitest";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const warningProneTestFiles = [
  "src/features/work-outcome/components/work-chart-warning-regression.test.tsx",
  "src/features/header/components/tick-slider-control.test.tsx",
];
const blockedWarningFragments = [
  "The width(-1) and height(-1) of chart should be greater than 0",
  "not wrapped in act",
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

function resolveRuntimeCommand() {
  if (hasBun()) {
    return {
      args: ["x", "vitest", "run", ...warningProneTestFiles],
      command: bunCommand(),
    };
  }

  return {
    args: ["exec", "--", "vitest", "run", ...warningProneTestFiles],
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

async function runWarningCleanupLane() {
  const runtime = resolveRuntimeCommand();
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

test(
  "warning-prone UI regression lane stays quiet in a real Vitest run",
  async () => {
    const result = await runWarningCleanupLane();
    const combinedOutput = `${result.stdout}\n${result.stderr}`;

    expect(result.exitCode).toBe(0);
    for (const blockedWarningFragment of blockedWarningFragments) {
      expect(combinedOutput).not.toContain(blockedWarningFragment);
    }
  },
  30_000,
);
