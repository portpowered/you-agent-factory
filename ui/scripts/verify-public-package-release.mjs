import { spawn } from "node:child_process";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const packageReleaseGates = [
  ["@you-agent-factory/client", "client", "verify"],
  ["@you-agent-factory/components", "components", "verify"],
  ["@you-agent-factory/factory-replay", "factory-replay", "verify"],
  ["@you-agent-factory/factory-emulator", "factory-emulator", "verify:release"],
  ["@you-agent-factory/factory-visualizers", "factory-visualizers", "verify"],
].map(([packageName, directoryName, releaseScript]) => ({
  packageName,
  packageDirectory: path.join(uiRoot, "packages", directoryName),
  releaseScript,
}));

const workspaceLinkStep = {
  packageName: "Public package family",
  packageDirectory: uiRoot,
  stepName: "link built workspace dependencies",
  command: "node",
  args: ["scripts/link-public-package-dependencies.mjs"],
};

const replayRetainedMemoryStep = {
  packageName: "@you-agent-factory/factory-replay consumer",
  packageDirectory: uiRoot,
  stepName: "run 10,000-event retained-memory regression",
  args: ["run", "test:public-package-retained-memory"],
};

export const PUBLIC_PACKAGE_RELEASE_STEPS = Object.freeze([
  {
    packageName: "Public package family",
    packageDirectory: uiRoot,
    stepName: "install locked UI prerequisites",
    args: ["install", "--frozen-lockfile"],
  },
  {
    packageName: "Public package family",
    packageDirectory: uiRoot,
    stepName: "run orchestration regression tests",
    args: ["run", "test:public-package-release"],
  },
  ...packageReleaseGates.flatMap((packageGate, index) => [
    {
      ...packageGate,
      stepName: "run release gate",
      args: ["run", packageGate.releaseScript],
    },
    ...(index === 2 ? [replayRetainedMemoryStep, workspaceLinkStep] : []),
  ]),
]);

function formatCommand(step) {
  return `${step.command ?? "bun"} ${step.args.join(" ")}`;
}

function formatExit(code, signal) {
  return code === null ? `signal ${signal ?? "unknown"}` : `exit code ${code}`;
}

export function runBunStep(step) {
  return new Promise((resolve, reject) => {
    const child = spawn(step.command ?? "bun", step.args, {
      cwd: step.packageDirectory,
      env: process.env,
      shell: false,
      stdio: "inherit",
    });

    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }

      reject(
        new Error(
          `${formatCommand(step)} finished with ${formatExit(code, signal)}`,
        ),
      );
    });
  });
}

export async function verifyPublicPackageRelease({
  steps = PUBLIC_PACKAGE_RELEASE_STEPS,
  runStep = runBunStep,
  log = console.log,
} = {}) {
  for (const step of steps) {
    const label = `${step.packageName}: ${step.stepName}`;
    log(`\n==> ${label}`);

    try {
      await runStep(step);
    } catch (error) {
      const outcome = error instanceof Error ? error.message : String(error);
      throw new Error(
        `Public package release failed at ${label} (${formatCommand(step)}): ${outcome}`,
        { cause: error },
      );
    }
  }

  log("\nAll five public package release gates passed.");
}

export async function main() {
  await verifyPublicPackageRelease();
}

const isMain =
  process.argv[1] !== undefined &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);

if (isMain) {
  try {
    await main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}
