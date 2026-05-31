#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  browserCoveragePathIgnorePatterns,
  coverageTestTimeoutMs,
  mainCoveragePathIgnorePatterns,
  mainCoverageReportsDir,
  reactFlowCoverageTestPath,
} from "./bun-coverage-config.mjs";
import {
  getMainCoveredMaxWorkers,
  parseUiCoverageShard,
} from "./ui-coverage-runner.mjs";

const uiRoot = fileURLToPath(new URL("..", import.meta.url));

const NODE_LANE_PATHS = [
  "./vite.config.test.ts",
  "./src/features/export/lib/browser-download.node.test.ts",
  "./src/features/export/lib/factory-png-export.node.test.ts",
  "./src/features/import/lib/factory-png-import.node.test.ts",
  "./scripts/check-hardcoded-ui-copy.test.ts",
  "./scripts/verify-test-output-cleanup.test.mjs",
];

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

function sanitizeBatchId(label) {
  return label.replace(/[^a-zA-Z0-9._-]+/g, "_");
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
    if (relativePath === "./src/features/export") {
      batches.push({
        label: "./src/features/export/hooks/use-current-factory-export.test.tsx",
        paths: ["./src/features/export/hooks/use-current-factory-export.test.tsx"],
      });
      batches.push({
        label: "./src/features/export/components/export-factory-dialog.test.tsx",
        paths: [
          "./src/features/export/components/export-factory-dialog.test.tsx",
        ],
      });
      batches.push({
        label: relativePath,
        paths: [relativePath],
        pathIgnorePatterns: [
          "**/use-current-factory-export.test.tsx",
          "**/export-factory-dialog.test.tsx",
        ],
      });
      continue;
    }

    batches.push({
      label: relativePath,
      paths: [relativePath],
      pathIgnorePatterns:
        relativePath === WORKFLOW_ACTIVITY_LANE
          ? [reactFlowCoverageTestPath]
          : undefined,
    });
  }

  batches.push({
    label: "./scripts (bun unit)",
    paths: BUN_UNIT_SCRIPT_PATHS,
  });

  return batches;
}

function buildMainCoverageWorkItems() {
  return [
    { nodeLane: true, batch: { label: "node-lane", paths: NODE_LANE_PATHS } },
    ...buildBrowserLaneBatches().map((batch) => ({ nodeLane: false, batch })),
  ];
}

function selectShardWorkItems(shard) {
  const workItems = buildMainCoverageWorkItems();
  if (!shard) {
    return workItems;
  }

  return workItems.filter(
    (_item, workItemIndex) => workItemIndex % shard.total === shard.index - 1,
  );
}

function resolveMainCoverageReportsDir(shard) {
  if (!shard) {
    return mainCoverageReportsDir;
  }

  return `${mainCoverageReportsDir}/main-shard-${shard.index}`;
}

function runBunCoverageBatch(
  bunCommand,
  batch,
  {
    nodeLane = false,
    parallelWorkers,
    reportsDir = mainCoverageReportsDir,
    spawn = spawnSync,
  } = {},
) {
  const coverageDir = `${reportsDir}/${sanitizeBatchId(batch.label)}`;
  const pathIgnorePatterns = nodeLane
    ? mainCoveragePathIgnorePatterns
    : browserCoveragePathIgnorePatterns;
  const args = [
    "test",
    "--timeout",
    String(coverageTestTimeoutMs),
    "--parallel",
    String(parallelWorkers),
    "--isolate",
    "--coverage",
    "--coverage-reporter=lcov",
    `--coverage-dir=${coverageDir}`,
  ];

  for (const pattern of pathIgnorePatterns) {
    args.push("--path-ignore-patterns", pattern);
  }

  if (batch.pathIgnorePatterns) {
    for (const pattern of batch.pathIgnorePatterns) {
      args.push("--path-ignore-patterns", pattern);
    }
  }

  if (nodeLane) {
    args.push("--config=bunfig.node.toml");
  }

  args.push(...batch.paths);

  return spawn(bunCommand, args, {
    stdio: "inherit",
    cwd: uiRoot,
    env: nodeLane
      ? { ...process.env, BUN_TEST_NODE_LANE: "1", VITEST: "true" }
      : process.env,
  });
}

function resolveBunCommand() {
  const versionResult = spawnSync("bun", ["--version"], { encoding: "utf8" });
  if (versionResult.status !== 0) {
    console.error("UI coverage requires Bun 1.3.12+ on PATH.");
    process.exit(versionResult.status ?? 1);
  }

  return "bun";
}

export function runMainCoveredBunPass(options = {}) {
  const bunCommand = options.bunCommand ?? resolveBunCommand();
  const env = options.env ?? process.env;
  const shard = parseUiCoverageShard(env);
  const parallelWorkers =
    options.mainCoveredMaxWorkers ??
    getMainCoveredMaxWorkers(env, { shard: Boolean(shard) });
  const spawn = options.spawn ?? spawnSync;
  const reportsDir = resolveMainCoverageReportsDir(shard);
  const workItems = selectShardWorkItems(shard);

  if (shard) {
    console.log(
      `[ui-coverage] Main covered Bun shard ${shard.label}: ${workItems.length} work item(s)`,
    );
  }

  for (const { nodeLane, batch } of workItems) {
    console.log(
      `[ui-coverage] Main covered Bun pass ${nodeLane ? "node" : "browser"} lane:`,
      batch.label,
    );
    const result = runBunCoverageBatch(bunCommand, batch, {
      nodeLane,
      parallelWorkers,
      reportsDir,
      spawn,
    });
    if (result.status !== 0) {
      return result.status ?? 1;
    }
  }

  return 0;
}

if (import.meta.url === new URL(process.argv[1], "file:").href) {
  process.exit(runMainCoveredBunPass());
}
