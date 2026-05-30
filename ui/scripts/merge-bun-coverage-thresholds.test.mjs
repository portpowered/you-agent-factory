import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { expect, test } from "vitest";

import { mergeAndCheckBunCoverage } from "./merge-bun-coverage-thresholds.mjs";

function writeLcov(rootDir, relativeDir, content) {
  const directory = join(rootDir, relativeDir);
  mkdirSync(directory, { recursive: true });
  writeFileSync(join(directory, "lcov.info"), content, "utf8");
}

test("merges Bun phase lcov files and enforces configured thresholds", async () => {
  const rootDir = mkdtempSync(join(tmpdir(), "bun-coverage-merge-"));
  const coveredSource = "src/components/ui/button.tsx";
  const lcov = [
    "TN:",
    `SF:${coveredSource}`,
    "FNF:2",
    "FNH:2",
    "LF:4",
    "LH:4",
    "BRF:2",
    "BRH:2",
    "DA:1,1",
    "DA:2,1",
    "DA:3,1",
    "DA:4,1",
    "end_of_record",
    "",
  ].join("\n");

  try {
    writeLcov(rootDir, ".bun-coverage-reports/main/batch-a", lcov);
    writeLcov(rootDir, ".bun-coverage-reports/react-flow-current-activity-card", lcov);

    expect(await mergeAndCheckBunCoverage({ rootDir })).toBe(0);
  } finally {
    rmSync(rootDir, { force: true, recursive: true });
  }
});

test("fails merge pass when merged coverage is below thresholds", async () => {
  const rootDir = mkdtempSync(join(tmpdir(), "bun-coverage-merge-fail-"));
  const coveredSource = "src/components/ui/button.tsx";
  const lcov = [
    "TN:",
    `SF:${coveredSource}`,
    "FNF:2",
    "FNH:0",
    "LF:4",
    "LH:1",
    "BRF:2",
    "BRH:0",
    "DA:1,1",
    "DA:2,0",
    "DA:3,0",
    "DA:4,0",
    "end_of_record",
    "",
  ].join("\n");

  try {
    writeLcov(rootDir, ".bun-coverage-reports/main/batch-a", lcov);

    expect(await mergeAndCheckBunCoverage({ rootDir })).toBe(1);
  } finally {
    rmSync(rootDir, { force: true, recursive: true });
  }
});
