import { readdir } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { allowlistedFeatureRootFiles } from "./feature-root-file-allowlist.mjs";
import { allowlistedFeatureRootSubdirectories } from "./feature-root-subdirectory-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const featureRootDir = process.env.AGENT_FACTORY_UI_FEATURES_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_FEATURES_DIR)
  : path.join(defaultUiDir, "src", "features");
const uiDir = path.dirname(path.dirname(featureRootDir));
const allowlistOverride =
  process.env.AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST;
const subdirectoryAllowlistOverride =
  process.env.AGENT_FACTORY_UI_FEATURE_ROOT_SUBDIRECTORY_ALLOWLIST;
const approvedFeatureRootSubdirectories = new Set([
  "components",
  "hooks",
  "lib",
  "messages",
  "public",
  "selectors",
  "state",
]);

function getConfiguredAllowlist() {
  if (allowlistOverride === undefined) {
    return allowlistedFeatureRootFiles;
  }

  return allowlistOverride
    .split(/\r?\n/)
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

function getConfiguredSubdirectoryAllowlist() {
  if (subdirectoryAllowlistOverride === undefined) {
    return allowlistedFeatureRootSubdirectories;
  }

  return subdirectoryAllowlistOverride
    .split(/\r?\n/)
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
}

function toUiRelativePath(filePath, rootDirectory = uiDir) {
  return toPosixPath(path.relative(rootDirectory, filePath));
}

function formatViolation(relativeFilePath, message) {
  return [relativeFilePath, `  ${message}`].join("\n");
}

export async function scanFeatureRootFiles(
  rootDirectory = featureRootDir,
  allowlist = allowlistedFeatureRootFiles,
  subdirectoryAllowlist = allowlistedFeatureRootSubdirectories,
) {
  const rootUiDirectory = path.dirname(path.dirname(rootDirectory));
  const allowlistedPaths = new Set(
    allowlist.map((filePath) => toPosixPath(filePath)),
  );
  const allowlistedSubdirectoryPaths = new Set(
    subdirectoryAllowlist.map((directoryPath) => toPosixPath(directoryPath)),
  );
  const observedAllowlistedPaths = new Set();
  const observedAllowlistedSubdirectoryPaths = new Set();
  const featureEntries = await readdir(rootDirectory, { withFileTypes: true });
  const allowlistedDebt = [];
  const allowlistedSubdirectoryDebt = [];
  const violations = [];
  const subdirectoryViolations = [];

  for (const featureEntry of featureEntries.sort((left, right) =>
    left.name.localeCompare(right.name),
  )) {
    if (!featureEntry.isDirectory()) {
      continue;
    }

    const featureDirectory = path.join(rootDirectory, featureEntry.name);
    const directChildren = await readdir(featureDirectory, {
      withFileTypes: true,
    });

    for (const childEntry of directChildren.sort((left, right) =>
      left.name.localeCompare(right.name),
    )) {
      if (childEntry.isDirectory()) {
        if (approvedFeatureRootSubdirectories.has(childEntry.name)) {
          continue;
        }

        const absoluteDirectoryPath = path.join(
          featureDirectory,
          childEntry.name,
        );
        const relativeDirectoryPath = toUiRelativePath(
          absoluteDirectoryPath,
          rootUiDirectory,
        );
        const record = {
          featureName: featureEntry.name,
          directoryPath: absoluteDirectoryPath,
          relativeDirectoryPath,
          subdirectoryName: childEntry.name,
        };

        if (allowlistedSubdirectoryPaths.has(relativeDirectoryPath)) {
          observedAllowlistedSubdirectoryPaths.add(relativeDirectoryPath);
          allowlistedSubdirectoryDebt.push(record);
          continue;
        }

        subdirectoryViolations.push(record);
        continue;
      }

      const absoluteFilePath = path.join(featureDirectory, childEntry.name);
      const relativeFilePath = toUiRelativePath(
        absoluteFilePath,
        rootUiDirectory,
      );
      const record = {
        featureName: featureEntry.name,
        filePath: absoluteFilePath,
        relativeFilePath,
      };

      if (allowlistedPaths.has(relativeFilePath)) {
        observedAllowlistedPaths.add(relativeFilePath);
        allowlistedDebt.push(record);
        continue;
      }

      violations.push(record);
    }
  }

  const staleAllowlistEntries = [...allowlistedPaths]
    .filter((filePath) => !observedAllowlistedPaths.has(filePath))
    .sort((left, right) => left.localeCompare(right));
  const staleSubdirectoryAllowlistEntries = [...allowlistedSubdirectoryPaths]
    .filter(
      (directoryPath) =>
        !observedAllowlistedSubdirectoryPaths.has(directoryPath),
    )
    .sort((left, right) => left.localeCompare(right));

  return {
    allowlistedDebt,
    allowlistedSubdirectoryDebt,
    staleAllowlistEntries,
    staleSubdirectoryAllowlistEntries,
    subdirectoryViolations,
    violations,
  };
}

function buildFeatureRootGuardReports(report) {
  const allowlistedDebtReport = report.allowlistedDebt
    .map((entry) =>
      formatViolation(
        entry.relativeFilePath,
        "Allowlisted legacy feature-root file. Migrate it under an approved subdirectory and remove the allowlist entry in the same change.",
      ),
    )
    .join("\n\n");
  const allowlistedSubdirectoryDebtReport = report.allowlistedSubdirectoryDebt
    .map((entry) =>
      formatViolation(
        entry.relativeDirectoryPath,
        "Allowlisted legacy feature-root domain subdirectory. Keep the subdivision under an approved lane when possible and remove the allowlist entry in the same change.",
      ),
    )
    .join("\n\n");
  const violationReport = report.violations
    .map((entry) =>
      formatViolation(
        entry.relativeFilePath,
        "Feature roots may contain directories only. Move this file under an approved subdirectory such as public/, components/, hooks/, messages/, state/, selectors/, lib/, or a more specific domain folder.",
      ),
    )
    .join("\n\n");
  const subdirectoryViolationReport = report.subdirectoryViolations
    .map((entry) =>
      formatViolation(
        entry.relativeDirectoryPath,
        "Feature roots may contain approved subdirectories only. Use public/, components/, hooks/, messages/, state/, selectors/, lib/, or a narrower domain folder with an explicit allowlist entry while legacy debt is retired.",
      ),
    )
    .join("\n\n");
  const staleAllowlistReport = report.staleAllowlistEntries
    .map((filePath) =>
      formatViolation(
        filePath,
        "Allowlist entry is stale. Remove it in the same change that deleted or relocated the feature-root file.",
      ),
    )
    .join("\n\n");
  const staleSubdirectoryAllowlistReport =
    report.staleSubdirectoryAllowlistEntries
      .map((directoryPath) =>
        formatViolation(
          directoryPath,
          "Allowlist entry is stale. Remove it in the same change that deleted or relocated the feature-root subdirectory.",
        ),
      )
      .join("\n\n");

  return {
    allowlistedDebtReport,
    allowlistedSubdirectoryDebtReport,
    staleAllowlistReport,
    staleSubdirectoryAllowlistReport,
    subdirectoryViolationReport,
    violationReport,
  };
}

function hasFeatureRootGuardFailures(report) {
  return (
    report.violations.length > 0 ||
    report.subdirectoryViolations.length > 0 ||
    report.staleAllowlistEntries.length > 0 ||
    report.staleSubdirectoryAllowlistEntries.length > 0
  );
}

async function main() {
  const report = await scanFeatureRootFiles(
    featureRootDir,
    getConfiguredAllowlist(),
    getConfiguredSubdirectoryAllowlist(),
  );
  const reports = buildFeatureRootGuardReports(report);

  if (hasFeatureRootGuardFailures(report)) {
    console.error(
      [
        "Feature root file guard failed.",
        "Each ui/src/features/<feature>/ directory may contain approved subdirectories only, with no root-level files.",
        reports.violationReport
          ? [
              "New hard-fail root-file violations:",
              reports.violationReport,
            ].join("\n\n")
          : "",
        reports.subdirectoryViolationReport
          ? [
              "New hard-fail root-subdirectory violations:",
              reports.subdirectoryViolationReport,
            ].join("\n\n")
          : "",
        reports.staleAllowlistReport.length > 0
          ? ["Stale allowlist entries:", reports.staleAllowlistReport].join(
              "\n\n",
            )
          : "",
        reports.staleSubdirectoryAllowlistReport.length > 0
          ? [
              "Stale subdirectory allowlist entries:",
              reports.staleSubdirectoryAllowlistReport,
            ].join("\n\n")
          : "",
        reports.allowlistedDebtReport.length > 0
          ? ["Allowlisted legacy debt:", reports.allowlistedDebtReport].join(
              "\n\n",
            )
          : "",
        reports.allowlistedSubdirectoryDebtReport.length > 0
          ? [
              "Allowlisted legacy subdirectory debt:",
              reports.allowlistedSubdirectoryDebtReport,
            ].join("\n\n")
          : "",
      ]
        .filter(Boolean)
        .join("\n\n"),
    );
    process.exitCode = 1;
    return;
  }

  if (
    reports.allowlistedDebtReport.length > 0 ||
    reports.allowlistedSubdirectoryDebtReport.length > 0
  ) {
    console.log(
      [
        "Feature root file guard passed with allowlisted legacy debt.",
        reports.allowlistedDebtReport
          ? [
              "Current allowlisted feature-root files:",
              reports.allowlistedDebtReport,
            ].join("\n\n")
          : "",
        reports.allowlistedSubdirectoryDebtReport
          ? [
              "Current allowlisted feature-root subdirectories:",
              reports.allowlistedSubdirectoryDebtReport,
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
