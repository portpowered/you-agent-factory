import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

import {
  categorizeUiTestFile,
  formatSlowFileSummaryLines,
  mergeFileDurations,
  parseVitestFileDurationsFromLog,
  rankSlowestTestFiles,
  uiTestCostCategories,
} from "./ui-test-cost-report.mjs";

const fixtureLogSnippet = readFileSync(
  join(
    dirname(fileURLToPath(import.meta.url)),
    "fixtures/vitest-main-pass-log-snippet.txt",
  ),
  "utf8",
);

test("defines stable UI test cost categories", () => {
  expect(uiTestCostCategories).toEqual([
    "app-shell-integration",
    "react-flow-graph",
    "replay-timeline",
    "import-export",
    "script-style",
    "uncategorized",
  ]);
});

test("categorizes covered and browser integration paths", () => {
  expect(categorizeUiTestFile("src/App.session-stream.test.tsx")).toBe(
    "app-shell-integration",
  );
  expect(
    categorizeUiTestFile(
      "src/features/workflow-activity/components/current-activity-card/react-flow-current-activity-card-editor-chrome.test.tsx",
    ),
  ).toBe("react-flow-graph");
  expect(
    categorizeUiTestFile(
      "src/features/timeline/state/factoryTimelineStore.test.ts",
    ),
  ).toBe("replay-timeline");
  expect(categorizeUiTestFile("src/App.import.test.tsx")).toBe("import-export");
  expect(
    categorizeUiTestFile(
      "scripts/dashboard-shell-storybook-responsive.test.mjs",
    ),
  ).toBe("script-style");
  expect(
    categorizeUiTestFile(
      "integration/factory-graph-editor.integration.test.mjs",
    ),
  ).toBe("react-flow-graph");
  expect(
    categorizeUiTestFile(
      "integration/event-stream-replay.integration.test.mjs",
    ),
  ).toBe("replay-timeline");
  expect(
    categorizeUiTestFile(
      "integration/factory-import-second-session.integration.test.mjs",
    ),
  ).toBe("import-export");
  expect(categorizeUiTestFile("src/api/baseUrl.test.ts")).toBe("uncategorized");
});

test("merges shard timing artifacts by max duration per file", () => {
  expect(
    mergeFileDurations([
      { durationMs: 1000, path: "src/a.test.ts" },
      { durationMs: 2500, path: "src/a.test.ts" },
      { durationMs: 500, path: "src/b.test.ts" },
    ]),
  ).toEqual([
    { durationMs: 2500, path: "src/a.test.ts" },
    { durationMs: 500, path: "src/b.test.ts" },
  ]);
});

test("formats browser integration slow-file summaries with categories", () => {
  const durations = rankSlowestTestFiles(
    parseVitestFileDurationsFromLog(fixtureLogSnippet),
    2,
  );

  expect(
    formatSlowFileSummaryLines(durations, {
      limit: 2,
      logPrefix: "[ui-browser-integration]",
      summaryTitle: "Browser integration slowest test files",
    }),
  ).toEqual([
    "[ui-browser-integration] Browser integration slowest test files (top 2):",
    "[ui-browser-integration]   src/App.test.tsx 120.00s [app-shell-integration]",
    "[ui-browser-integration]   src/features/timeline/state/factoryTimelineStore.test.ts 5.00s [replay-timeline]",
  ]);
});

test("aggregates verbose parallel test timings when Vitest omits file timing lines", () => {
  expect(
    parseVitestFileDurationsFromLog(
      [
        " ✓ integration/a.integration.test.mjs > scenario > first 125ms",
        " ✓ integration/a.integration.test.mjs > scenario > second 75ms",
        " ✓ integration/b.integration.test.mjs > scenario > only 40ms",
      ].join("\n"),
    ),
  ).toEqual([
    { durationMs: 200, path: "integration/a.integration.test.mjs" },
    { durationMs: 40, path: "integration/b.integration.test.mjs" },
  ]);
});
