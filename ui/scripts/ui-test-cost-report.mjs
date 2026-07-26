export const uiTestCostCategories = [
  "app-shell-integration",
  "react-flow-graph",
  "replay-timeline",
  "import-export",
  "script-style",
  "uncategorized",
];

const vitestFileDurationLinePattern =
  /^\s*[✓×]\s+(\S+\.(?:test|spec)\.(?:tsx?|mjs|cjs))\s+\([^)]+\)\s+(\d+(?:\.\d+)?)ms(?:\s+\d+ MB heap used)?/gm;
const vitestVerboseTestDurationLinePattern =
  /^\s*[✓×]\s+(\S+\.(?:test|spec)\.(?:tsx?|mjs|cjs))\s+>.+\s+(\d+(?:\.\d+)?)ms$/gm;

const ansiEscapePattern = new RegExp(
  `${String.fromCharCode(27)}\\[[0-9;]*m`,
  "g",
);

export const defaultSlowFileSummaryLimit = 15;
export const defaultCapturedStdoutMaxBuffer = 128 * 1024 * 1024;

export function stripAnsi(text) {
  return text.replace(ansiEscapePattern, "");
}

export function parseVitestFileDurationsFromLog(logText) {
  const strippedLog = stripAnsi(logText);
  const durationsByPath = new Map();
  const verboseDurationsByPath = new Map();

  for (const match of strippedLog.matchAll(vitestFileDurationLinePattern)) {
    const [, filePath, durationMsText] = match;
    const durationMs = Number(durationMsText);
    durationsByPath.set(
      filePath,
      Math.max(durationsByPath.get(filePath) ?? 0, durationMs),
    );
  }

  for (const match of strippedLog.matchAll(
    vitestVerboseTestDurationLinePattern,
  )) {
    const [, filePath, durationMsText] = match;
    verboseDurationsByPath.set(
      filePath,
      (verboseDurationsByPath.get(filePath) ?? 0) + Number(durationMsText),
    );
  }

  for (const [filePath, durationMs] of verboseDurationsByPath) {
    if (!durationsByPath.has(filePath)) {
      durationsByPath.set(filePath, durationMs);
    }
  }

  return [...durationsByPath.entries()].map(([path, durationMs]) => ({
    path,
    durationMs,
  }));
}

export function categorizeUiTestFile(filePath) {
  const normalized = filePath.replace(/\\/g, "/");

  if (
    normalized.includes("/scripts/") &&
    (normalized.endsWith(".test.mjs") || normalized.endsWith(".test.ts"))
  ) {
    return "script-style";
  }
  if (normalized.includes("dashboard-shell-storybook-responsive")) {
    return "script-style";
  }

  if (
    normalized.startsWith("integration/") ||
    normalized.includes("/integration/")
  ) {
    if (
      normalized.includes("import") ||
      normalized.includes("export") ||
      normalized.includes("name-preservation")
    ) {
      return "import-export";
    }
    if (normalized.includes("replay") || normalized.includes("event-stream")) {
      return "replay-timeline";
    }
    if (normalized.includes("graph-editor") || normalized.includes("graph")) {
      return "react-flow-graph";
    }
    if (
      normalized.includes("session-tabs") ||
      normalized.includes("page-shell") ||
      normalized.includes("header-palette") ||
      normalized.includes("phantom-worker")
    ) {
      return "app-shell-integration";
    }
    return "uncategorized";
  }

  if (/src\/App(?:\.[^/]+)?\.test\.tsx$/.test(normalized)) {
    if (normalized.includes("import") || normalized.includes("export")) {
      return "import-export";
    }
    return "app-shell-integration";
  }
  if (
    normalized.includes("dashboard-replay-wiring.component.test.tsx") ||
    normalized.includes("dashboard-trace-wiring.component.test.tsx") ||
    normalized.includes(
      "dashboard-session-timeline-isolation.component.test.tsx",
    ) ||
    normalized.includes(
      "use-dashboard-snapshot-checkpoint-lifecycle.component.test.tsx",
    )
  ) {
    return "app-shell-integration";
  }
  if (
    normalized.includes("react-flow") ||
    normalized.includes("workflow-activity/components/react-flow")
  ) {
    return "react-flow-graph";
  }
  if (normalized.includes("timeline") || normalized.includes("replay")) {
    return "replay-timeline";
  }
  if (normalized.includes("import") || normalized.includes("export")) {
    return "import-export";
  }

  return "uncategorized";
}

export function mergeFileDurations(fileDurations) {
  const durationsByPath = new Map();

  for (const { path, durationMs } of fileDurations) {
    durationsByPath.set(
      path,
      Math.max(durationsByPath.get(path) ?? 0, durationMs),
    );
  }

  return [...durationsByPath.entries()].map(([path, durationMs]) => ({
    path,
    durationMs,
  }));
}

export function rankSlowestTestFiles(
  fileDurations,
  limit = defaultSlowFileSummaryLimit,
) {
  return [...fileDurations]
    .sort((left, right) => right.durationMs - left.durationMs)
    .slice(0, limit);
}

export function formatElapsedMs(elapsedMs) {
  return `${(elapsedMs / 1000).toFixed(2)}s`;
}

export function formatSlowFileSummaryLines(
  slowFiles,
  { limit = defaultSlowFileSummaryLimit, logPrefix, summaryTitle },
) {
  if (slowFiles.length === 0) {
    return [`${logPrefix} ${summaryTitle}: none reported`];
  }

  const lines = [
    `${logPrefix} ${summaryTitle} (top ${Math.min(slowFiles.length, limit)}):`,
  ];

  for (const { path, durationMs } of slowFiles) {
    const category = categorizeUiTestFile(path);
    lines.push(
      `${logPrefix}   ${path} ${formatElapsedMs(durationMs)} [${category}]`,
    );
  }

  return lines;
}

export function logSlowFileSummary(capturedStdout, options) {
  const slowFiles = rankSlowestTestFiles(
    parseVitestFileDurationsFromLog(capturedStdout),
    options.limit,
  );

  for (const line of formatSlowFileSummaryLines(slowFiles, options)) {
    console.log(line);
  }
}
