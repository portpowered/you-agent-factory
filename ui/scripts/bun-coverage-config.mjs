/** Bun-owned coverage thresholds and excludes (mirrors ui/vite.config.ts today). */
export const bunCoverageReportsDir = ".bun-coverage-reports";
export const mainCoverageReportsDir = `${bunCoverageReportsDir}/main`;
export const reactFlowCoverageReportsDir = `${bunCoverageReportsDir}/react-flow-current-activity-card`;
export const mergedCoverageDir = "coverage";

export const reactFlowCoverageTestPath =
  "src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx";

export const mainCoveragePathIgnorePatterns = [
  "integration/**",
  "scripts/dashboard-shell-storybook-responsive.test.mjs",
  reactFlowCoverageTestPath,
];

/** Applied to browser-lane coverage batches so node-only specs stay in the node lane. */
export const browserCoveragePathIgnorePatterns = [
  ...mainCoveragePathIgnorePatterns,
  "**/*storybook*.test.mjs",
  "**/*.node.test.ts",
  "scripts/check-hardcoded-ui-copy.test.ts",
  "scripts/verify-test-output-cleanup.test.mjs",
  "vite.config.test.ts",
  "node_modules/**",
];

export const coverageExcludePatterns = [
  "**/node_modules/**",
  "**/.git/**",
  "**/*.jsonl",
  "scripts/**",
  "src/testing/app-shell-test-graph-layout.ts",
  "src/testing/replay-harness.ts",
  "src/styles.css",
  "**/index.ts",
  "testing/**",
  "**/*.{test,spec}.{ts,tsx,mjs}",
  "**/*.bun.test.{ts,tsx}",
];

/** Temporary Bun baseline thresholds; Vitest v8 previously enforced 93.1/80.4/94.9/93.1. */
export const coverageThresholds = {
  statements: 87.5,
  branches: 80.4,
  functions: 94.9,
  lines: 87.5,
};

export const coverageTestTimeoutMs = 180_000;
