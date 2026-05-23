import { readdir } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { allowlistedFeatureRootFiles } from "./feature-root-file-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const featureRootDir = process.env.AGENT_FACTORY_UI_FEATURES_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_FEATURES_DIR)
  : path.join(defaultUiDir, "src", "features");
const uiDir = path.dirname(path.dirname(featureRootDir));
const allowlistOverride = process.env.AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST;

function getConfiguredAllowlist() {
  if (!allowlistOverride) {
    return allowlistedFeatureRootFiles;
  }

  return allowlistOverride
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
) {
  const rootUiDirectory = path.dirname(path.dirname(rootDirectory));
  const allowlistedPaths = new Set(allowlist.map((filePath) => toPosixPath(filePath)));
  const observedAllowlistedPaths = new Set();
  const featureEntries = await readdir(rootDirectory, { withFileTypes: true });
  const allowlistedDebt = [];
  const violations = [];

  for (const featureEntry of featureEntries.sort((left, right) => left.name.localeCompare(right.name))) {
    if (!featureEntry.isDirectory()) {
      continue;
    }

    const featureDirectory = path.join(rootDirectory, featureEntry.name);
    const directChildren = await readdir(featureDirectory, { withFileTypes: true });

    for (const childEntry of directChildren.sort((left, right) => left.name.localeCompare(right.name))) {
      if (!childEntry.isFile()) {
        continue;
      }

      const absoluteFilePath = path.join(featureDirectory, childEntry.name);
      const relativeFilePath = toUiRelativePath(absoluteFilePath, rootUiDirectory);
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

  return {
    allowlistedDebt,
    staleAllowlistEntries,
    violations,
  };
}

async function main() {
  const report = await scanFeatureRootFiles(featureRootDir, getConfiguredAllowlist());
  const allowlistedDebtReport = report.allowlistedDebt
    .map((entry) =>
      formatViolation(
        entry.relativeFilePath,
        "Allowlisted legacy feature-root file. Migrate it under an approved subdirectory and remove the allowlist entry in the same change.",
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

  if (report.violations.length > 0 || report.staleAllowlistEntries.length > 0) {
    const violationReport = report.violations
      .map((entry) =>
        formatViolation(
          entry.relativeFilePath,
          "Feature roots may contain directories only. Move this file under an approved subdirectory such as public/, components/, hooks/, messages/, state/, selectors/, lib/, or a more specific domain folder.",
        ),
      )
      .join("\n\n");

    console.error(
      [
        "Feature root file guard failed.",
        "Each ui/src/features/<feature>/ directory may contain subdirectories only, with no root-level files.",
        "New hard-fail violations:",
        violationReport,
        staleAllowlistReport.length > 0 ? ["Stale allowlist entries:", staleAllowlistReport].join("\n\n") : "",
        allowlistedDebtReport.length > 0 ? ["Allowlisted legacy debt:", allowlistedDebtReport].join("\n\n") : "",
      ]
        .filter(Boolean)
        .join("\n\n"),
    );
    process.exitCode = 1;
    return;
  }

  if (allowlistedDebtReport.length > 0) {
    console.log(
      [
        "Feature root file guard passed with allowlisted legacy debt.",
        "Current allowlisted feature-root files:",
        allowlistedDebtReport,
      ].join("\n\n"),
    );
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
