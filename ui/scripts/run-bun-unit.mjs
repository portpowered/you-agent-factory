#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

const MIN_BUN_VERSION = [1, 3, 12];
const uiRoot = fileURLToPath(new URL("..", import.meta.url));

const NODE_LANE_PATHS = [
  "./vite.config.test.ts",
  "./src/features/export/lib/browser-download.node.test.ts",
  "./src/features/export/lib/factory-png-export.node.test.ts",
  "./src/features/import/lib/factory-png-import.node.test.ts",
  "./scripts/check-hardcoded-ui-copy.test.ts",
  "./scripts/verify-test-output-cleanup.test.mjs",
];

/** Bun unit script lane: static guards only; Storybook verifiers stay on Vitest. */
const BUN_UNIT_SCRIPT_PATHS = [
  "./scripts/check-button-usage.test.mjs",
  "./scripts/check-feature-root-files.test.mjs",
  "./scripts/check-inline-component-class-usage.test.mjs",
  "./scripts/check-semantic-color-tokens.test.ts",
  "./scripts/check-tailwind-spacing-tokens.test.mjs",
  "./scripts/normalize-dist-output.test.mjs",
  "./scripts/verify-current-selection-workstation-detail-order.test.mjs",
];

const WORKFLOW_ACTIVITY_LANE = "./src/features/workflow-activity";
const WORKFLOW_ACTIVITY_TIMEOUT_MS = 120_000;
const DEFAULT_TIMEOUT_MS = 30_000;

function parseBunVersion(rawVersion) {
  const match = /^(\d+)\.(\d+)\.(\d+)/.exec(rawVersion.trim());
  if (!match) {
    return null;
  }

  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

function isAtLeastVersion(version, minimum) {
  for (let index = 0; index < minimum.length; index += 1) {
    const current = version[index] ?? 0;
    const required = minimum[index] ?? 0;
    if (current > required) {
      return true;
    }
    if (current < required) {
      return false;
    }
  }

  return true;
}

function resolveBunCommand() {
  const bunFromPath = spawnSync("command", ["-v", "bun"], {
    encoding: "utf8",
    shell: true,
  });

  if (bunFromPath.status !== 0 || !bunFromPath.stdout.trim()) {
    console.error(
      "UI unit tests require Bun 1.3.12+ on PATH. Install from https://bun.sh and retry.",
    );
    process.exit(1);
  }

  const versionResult = spawnSync("bun", ["--version"], {
    encoding: "utf8",
  });
  if (versionResult.status !== 0) {
    console.error("Unable to read Bun version. Install Bun 1.3.12+ from https://bun.sh.");
    process.exit(versionResult.status ?? 1);
  }

  const version = parseBunVersion(versionResult.stdout);
  if (!version || !isAtLeastVersion(version, MIN_BUN_VERSION)) {
    console.error(
      `UI unit tests require Bun ${MIN_BUN_VERSION.join(".")}+ (found ${versionResult.stdout.trim()}).`,
    );
    process.exit(1);
  }

  return "bun";
}

function buildBrowserLaneBatches() {
  const batches = [];
  const srcDir = `${uiRoot}/src`;

  for (const name of readdirSync(srcDir)) {
    if (/^App\..*\.test\.tsx$/.test(name)) {
      batches.push({
        label: `./src/${name}`,
        paths: [`./src/${name}`],
      });
    }
  }

  for (const relativePath of [
    "./src/api",
    "./src/components",
    "./src/i18n",
    "./src/lib",
    "./src/testing",
  ]) {
    batches.push({ label: relativePath, paths: [relativePath] });
  }

  const featuresDir = `${srcDir}/features`;
  for (const dirent of readdirSync(featuresDir, { withFileTypes: true })) {
    if (!dirent.isDirectory()) {
      continue;
    }

    const relativePath = `./src/features/${dirent.name}`;
    batches.push({
      label: relativePath,
      paths: [relativePath],
      timeoutMs:
        relativePath === WORKFLOW_ACTIVITY_LANE
          ? WORKFLOW_ACTIVITY_TIMEOUT_MS
          : DEFAULT_TIMEOUT_MS,
    });
  }

  batches.push({
    label: "./scripts (bun unit)",
    paths: BUN_UNIT_SCRIPT_PATHS,
  });
  return batches;
}

function runBunTest(
  bunCommand,
  paths,
  { nodeLane = false, timeoutMs = DEFAULT_TIMEOUT_MS } = {},
) {
  const args = [
    "test",
    "--timeout",
    String(timeoutMs),
    "--parallel",
    "1",
    "--isolate",
  ];
  if (nodeLane) {
    args.push("--config=bunfig.node.toml");
  }
  args.push(...paths);

  return spawnSync(bunCommand, args, {
    stdio: "inherit",
    cwd: uiRoot,
    env: nodeLane
      ? { ...process.env, BUN_TEST_NODE_LANE: "1", VITEST: "true" }
      : process.env,
  });
}

const bunCommand = resolveBunCommand();

console.log("[ui-unit] node lane:", NODE_LANE_PATHS.join(", "));
const nodeResult = runBunTest(bunCommand, NODE_LANE_PATHS, { nodeLane: true });
if (nodeResult.status !== 0) {
  process.exit(nodeResult.status ?? 1);
}

const browserLaneBatches = buildBrowserLaneBatches();
for (const batch of browserLaneBatches) {
  console.log("[ui-unit] browser lane:", batch.label);
  const browserResult = runBunTest(bunCommand, batch.paths, {
    timeoutMs: batch.timeoutMs ?? DEFAULT_TIMEOUT_MS,
  });
  if (browserResult.status !== 0) {
    process.exit(browserResult.status ?? 1);
  }
}
