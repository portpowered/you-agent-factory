#!/usr/bin/env node
import { mkdirSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { mergeCoverageReportFiles } from "lcov-result-merger";
import { minimatch } from "minimatch";
import {
  bunCoverageReportsDir,
  coverageExcludePatterns,
  coverageThresholds,
  mergedCoverageDir,
} from "./bun-coverage-config.mjs";

function getUiRoot() {
  return fileURLToPath(new URL("..", import.meta.url));
}

function collectLcovFiles(directory) {
  const files = [];

  function walk(currentPath) {
    for (const entry of readdirSync(currentPath)) {
      const fullPath = join(currentPath, entry);
      const stats = statSync(fullPath);
      if (stats.isDirectory()) {
        walk(fullPath);
        continue;
      }

      if (entry === "lcov.info") {
        files.push(fullPath);
      }
    }
  }

  walk(directory);
  return files;
}

function normalizeCoveragePath(filePath) {
  return filePath.replaceAll("\\", "/").replace(/^\.\//, "");
}

function isExcluded(filePath) {
  const normalized = normalizeCoveragePath(filePath);
  return coverageExcludePatterns.some((pattern) =>
    minimatch(normalized, pattern, { dot: true }),
  );
}

function parseLcovRecords(lcovContent) {
  const records = [];
  const blocks = lcovContent.split("end_of_record");

  for (const block of blocks) {
    const lines = block
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    if (lines.length === 0) {
      continue;
    }

    const sourceFileLine = lines.find((line) => line.startsWith("SF:"));
    if (!sourceFileLine) {
      continue;
    }

    const sourceFile = sourceFileLine.slice(3);
    const record = {
      sourceFile,
      linesFound: 0,
      linesHit: 0,
      branchesFound: 0,
      branchesHit: 0,
      functionsFound: 0,
      functionsHit: 0,
      statementsFound: 0,
      statementsHit: 0,
    };

    for (const line of lines) {
      if (line.startsWith("LF:")) {
        record.linesFound = Number(line.slice(3));
      } else if (line.startsWith("LH:")) {
        record.linesHit = Number(line.slice(3));
      } else if (line.startsWith("BRF:")) {
        record.branchesFound = Number(line.slice(4));
      } else if (line.startsWith("BRH:")) {
        record.branchesHit = Number(line.slice(4));
      } else if (line.startsWith("FNF:")) {
        record.functionsFound = Number(line.slice(4));
      } else if (line.startsWith("FNH:")) {
        record.functionsHit = Number(line.slice(4));
      } else if (line.startsWith("DA:")) {
        record.statementsFound += 1;
        const [, hits] = line.slice(3).split(",");
        if (Number(hits) > 0) {
          record.statementsHit += 1;
        }
      }
    }

    if (record.linesFound === 0 && record.statementsFound > 0) {
      record.linesFound = record.statementsFound;
      record.linesHit = record.statementsHit;
    }

    records.push(record);
  }

  return records;
}

function percent(hit, total) {
  if (total === 0) {
    return 100;
  }

  return (hit / total) * 100;
}

function formatPercent(value) {
  return value.toFixed(2);
}

function summarizeCoverage(records) {
  const totals = {
    linesFound: 0,
    linesHit: 0,
    branchesFound: 0,
    branchesHit: 0,
    functionsFound: 0,
    functionsHit: 0,
    statementsFound: 0,
    statementsHit: 0,
  };

  for (const record of records) {
    if (isExcluded(record.sourceFile)) {
      continue;
    }

    totals.linesFound += record.linesFound;
    totals.linesHit += record.linesHit;
    totals.branchesFound += record.branchesFound;
    totals.branchesHit += record.branchesHit;
    totals.functionsFound += record.functionsFound;
    totals.functionsHit += record.functionsHit;
    totals.statementsFound += record.statementsFound;
    totals.statementsHit += record.statementsHit;
  }

  return {
    ...totals,
    lines: percent(totals.linesHit, totals.linesFound),
    branches: percent(totals.branchesHit, totals.branchesFound),
    functions: percent(totals.functionsHit, totals.functionsFound),
    statements: percent(totals.statementsHit, totals.statementsFound),
  };
}

function checkThresholds(summary) {
  const failures = [];
  const skipped = [];

  for (const [metric, minimum] of Object.entries(coverageThresholds)) {
    const totalKey =
      metric === "statements"
        ? "statementsFound"
        : metric === "branches"
          ? "branchesFound"
          : metric === "functions"
            ? "functionsFound"
            : "linesFound";

    if (summary[totalKey] === 0) {
      skipped.push(metric);
      continue;
    }

    const actual = summary[metric];
    if (actual + 1e-9 < minimum) {
      failures.push(
        `${metric}: ${formatPercent(actual)}% (required ${formatPercent(minimum)}%)`,
      );
    }
  }

  if (skipped.length > 0) {
    console.log(
      `Coverage thresholds skipped for metrics with no Bun lcov data: ${skipped.join(", ")}`,
    );
  }

  return failures;
}

export async function mergeAndCheckBunCoverage(options = {}) {
  const rootDir = options.rootDir ?? getUiRoot();
  const reportsDir = join(rootDir, bunCoverageReportsDir);
  const lcovFiles = collectLcovFiles(reportsDir);

  if (lcovFiles.length === 0) {
    throw new Error(
      `No lcov.info files found under ${relative(rootDir, reportsDir) || reportsDir}`,
    );
  }

  const mergedLcov = await mergeCoverageReportFiles(lcovFiles);
  const mergedPath = join(rootDir, mergedCoverageDir, "lcov.info");
  mkdirSync(dirname(mergedPath), { recursive: true });
  writeFileSync(mergedPath, mergedLcov, "utf8");

  const summary = summarizeCoverage(parseLcovRecords(mergedLcov));
  console.log("Coverage summary (Bun merged lcov):");
  console.log(
    `  statements: ${formatPercent(summary.statements)}% (${summary.statementsHit}/${summary.statementsFound})`,
  );
  console.log(
    `  branches: ${formatPercent(summary.branches)}% (${summary.branchesHit}/${summary.branchesFound})`,
  );
  console.log(
    `  functions: ${formatPercent(summary.functions)}% (${summary.functionsHit}/${summary.functionsFound})`,
  );
  console.log(
    `  lines: ${formatPercent(summary.lines)}% (${summary.linesHit}/${summary.linesFound})`,
  );

  const failures = checkThresholds(summary);
  if (failures.length > 0) {
    console.error("Coverage thresholds not met:");
    for (const failure of failures) {
      console.error(`  - ${failure}`);
    }
    return 1;
  }

  return 0;
}

if (import.meta.url === new URL(process.argv[1], "file:").href) {
  const status = await mergeAndCheckBunCoverage();
  process.exit(status);
}
