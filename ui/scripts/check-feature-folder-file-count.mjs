import { readdir, stat } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { allowlistedOversizedFeatureFolders } from "./feature-folder-file-count-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const defaultSrcDir = path.join(defaultUiDir, "src");
const featuresDir = process.env.AGENT_FACTORY_UI_FEATURES_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_FEATURES_DIR)
  : path.join(defaultSrcDir, "features");
const srcDir = path.dirname(featuresDir);
const uiDir = path.dirname(srcDir);
const folderFileLimit = 10;
const allowlistOverride =
  process.env.AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST;

function getConfiguredAllowlist() {
  if (allowlistOverride === undefined) {
    return allowlistedOversizedFeatureFolders;
  }

  return JSON.parse(allowlistOverride);
}

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
}

function toUiRelativePath(filePath, rootDirectory = uiDir) {
  return toPosixPath(path.relative(rootDirectory, filePath));
}

function formatViolation(relativeDirectoryPath, message) {
  return [relativeDirectoryPath, `  ${message}`].join("\n");
}

async function countDirectFiles(directoryPath) {
  const entries = await readdir(directoryPath, { withFileTypes: true });
  let fileCount = 0;

  for (const entry of entries) {
    if (entry.isFile()) {
      fileCount += 1;
    }
  }

  return fileCount;
}

async function collectFeatureDirectories(
  directoryPath,
  rootUiDirectory = uiDir,
  directories = [],
) {
  const entries = await readdir(directoryPath, { withFileTypes: true });

  for (const entry of entries) {
    if (!entry.isDirectory()) {
      continue;
    }

    const absoluteDirectoryPath = path.join(directoryPath, entry.name);
    const relativeDirectoryPath = toUiRelativePath(
      absoluteDirectoryPath,
      rootUiDirectory,
    );
    const fileCount = await countDirectFiles(absoluteDirectoryPath);

    directories.push({
      absoluteDirectoryPath,
      fileCount,
      relativeDirectoryPath,
    });
    await collectFeatureDirectories(
      absoluteDirectoryPath,
      rootUiDirectory,
      directories,
    );
  }

  return directories;
}

function buildAllowlistMap(allowlist) {
  const allowlistByPath = new Map();

  for (const entry of allowlist) {
    allowlistByPath.set(
      toPosixPath(entry.relativeDirectoryPath),
      entry.maxFileCount,
    );
  }

  return allowlistByPath;
}

export async function scanFeatureFolderFileCount(
  rootDirectory = featuresDir,
  allowlist = allowlistedOversizedFeatureFolders,
) {
  const rootUiDirectory = path.dirname(path.dirname(rootDirectory));
  const allowlistByPath = buildAllowlistMap(allowlist);
  const observedAllowlistedPaths = new Set();
  const allowlistedDebt = [];
  const growthViolations = [];
  const violations = [];

  let directories = [];
  try {
    const featuresRootStat = await stat(rootDirectory);
    if (featuresRootStat.isDirectory()) {
      directories = await collectFeatureDirectories(
        rootDirectory,
        rootUiDirectory,
      );
    }
  } catch (error) {
    if (error?.code !== "ENOENT") {
      throw error;
    }
  }

  for (const directory of directories.sort((left, right) =>
    left.relativeDirectoryPath.localeCompare(right.relativeDirectoryPath),
  )) {
    if (directory.fileCount <= folderFileLimit) {
      continue;
    }

    const allowlistedMaxFileCount = allowlistByPath.get(
      directory.relativeDirectoryPath,
    );
    const record = {
      ...directory,
      allowlistedMaxFileCount,
    };

    if (allowlistedMaxFileCount === undefined) {
      violations.push(record);
      continue;
    }

    observedAllowlistedPaths.add(directory.relativeDirectoryPath);

    if (directory.fileCount > allowlistedMaxFileCount) {
      growthViolations.push(record);
      continue;
    }

    allowlistedDebt.push(record);
  }

  const staleAllowlistEntries = [...allowlistByPath.keys()]
    .filter((directoryPath) => !observedAllowlistedPaths.has(directoryPath))
    .sort((left, right) => left.localeCompare(right));

  return {
    allowlistedDebt,
    folderFileLimit,
    growthViolations,
    staleAllowlistEntries,
    violations,
  };
}

function buildFeatureFolderFileCountReports(report) {
  const allowlistedDebtReport = report.allowlistedDebt
    .map((entry) =>
      formatViolation(
        entry.relativeDirectoryPath,
        `Allowlisted legacy oversized folder (${entry.fileCount} files, limit ${report.folderFileLimit}). Split this folder into smaller approved subdirectories and remove the allowlist entry in the same change when possible.`,
      ),
    )
    .join("\n\n");
  const violationReport = report.violations
    .map((entry) =>
      formatViolation(
        entry.relativeDirectoryPath,
        `Feature-owned folders may contain at most ${report.folderFileLimit} files. This folder has ${entry.fileCount} files. Split it into smaller approved subdirectories instead of growing the folder further.`,
      ),
    )
    .join("\n\n");
  const growthViolationReport = report.growthViolations
    .map((entry) =>
      formatViolation(
        entry.relativeDirectoryPath,
        `Allowlisted oversized folder grew from ${entry.allowlistedMaxFileCount} to ${entry.fileCount} files (limit ${report.folderFileLimit}). Split the folder into smaller approved subdirectories and remove or shrink the allowlist debt in the same change.`,
      ),
    )
    .join("\n\n");
  const staleAllowlistReport = report.staleAllowlistEntries
    .map((directoryPath) =>
      formatViolation(
        directoryPath,
        "Allowlist entry is stale. Remove it in the same change that reduced or reshaped this folder below the file-count limit.",
      ),
    )
    .join("\n\n");

  return {
    allowlistedDebtReport,
    growthViolationReport,
    staleAllowlistReport,
    violationReport,
  };
}

function hasFeatureFolderFileCountFailures(report) {
  return (
    report.violations.length > 0 ||
    report.growthViolations.length > 0 ||
    report.staleAllowlistEntries.length > 0
  );
}

async function main() {
  const report = await scanFeatureFolderFileCount(
    featuresDir,
    getConfiguredAllowlist(),
  );
  const reports = buildFeatureFolderFileCountReports(report);

  if (hasFeatureFolderFileCountFailures(report)) {
    console.error(
      [
        "Feature folder file-count guard failed.",
        `Each folder under ui/src/features may contain at most ${report.folderFileLimit} direct files.`,
        reports.violationReport
          ? ["New oversized folder violations:", reports.violationReport].join(
              "\n\n",
            )
          : "",
        reports.growthViolationReport
          ? [
              "Allowlisted folder growth violations:",
              reports.growthViolationReport,
            ].join("\n\n")
          : "",
        reports.staleAllowlistReport.length > 0
          ? ["Stale allowlist entries:", reports.staleAllowlistReport].join(
              "\n\n",
            )
          : "",
        reports.allowlistedDebtReport.length > 0
          ? ["Allowlisted legacy debt:", reports.allowlistedDebtReport].join(
              "\n\n",
            )
          : "",
      ]
        .filter(Boolean)
        .join("\n\n"),
    );
    process.exitCode = 1;
    return;
  }

  if (reports.allowlistedDebtReport.length > 0) {
    console.log(
      [
        "Feature folder file-count guard passed with allowlisted legacy debt.",
        reports.allowlistedDebtReport
          ? [
              "Current allowlisted oversized folders:",
              reports.allowlistedDebtReport,
            ].join("\n\n")
          : "",
      ]
        .filter(Boolean)
        .join("\n\n"),
    );
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
