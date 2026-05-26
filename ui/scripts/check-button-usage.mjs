import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import ts from "typescript";
import { fileURLToPath } from "node:url";

import { approvedButtonUsageAllowlist } from "./button-usage-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const sourceDir = process.env.AGENT_FACTORY_UI_SRC_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_SRC_DIR)
  : path.join(defaultUiDir, "src");
const uiDir = path.dirname(sourceDir);
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx"]);
const skippedFileSuffixes = [".test.js", ".test.jsx", ".test.ts", ".test.tsx", ".stories.tsx"];
const skippedDirectoryNames = new Set(["generated"]);
const skippedPathFragments = [`${path.sep}api${path.sep}generated${path.sep}`];
const allowlistOverride = process.env.AGENT_FACTORY_UI_BUTTON_USAGE_ALLOWLIST;

function getConfiguredAllowlist() {
  if (!allowlistOverride) {
    return approvedButtonUsageAllowlist;
  }

  return JSON.parse(allowlistOverride);
}

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
}

function toUiRelativePath(filePath, rootDirectory = uiDir) {
  return toPosixPath(path.relative(rootDirectory, filePath));
}

function shouldSkipFile(filePath) {
  if (!sourceExtensions.has(path.extname(filePath))) {
    return true;
  }

  if (skippedFileSuffixes.some((suffix) => filePath.endsWith(suffix))) {
    return true;
  }

  return skippedPathFragments.some((fragment) => filePath.includes(fragment));
}

async function collectSourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      if (skippedDirectoryNames.has(entry.name)) {
        continue;
      }

      files.push(...(await collectSourceFiles(entryPath)));
      continue;
    }

    if (!shouldSkipFile(entryPath)) {
      files.push(entryPath);
    }
  }

  return files;
}

function getScriptKind(filePath) {
  const extension = path.extname(filePath);

  if (extension === ".tsx") {
    return ts.ScriptKind.TSX;
  }

  if (extension === ".jsx") {
    return ts.ScriptKind.JSX;
  }

  if (extension === ".ts") {
    return ts.ScriptKind.TS;
  }

  return ts.ScriptKind.JS;
}

function indexToPosition(sourceFile, start) {
  const { character, line } = sourceFile.getLineAndCharacterOfPosition(start);
  return { column: character + 1, line: line + 1 };
}

function collectButtonUsage(sourceText, filePath) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const rawButtons = [];
  const buttonVariantsCalls = [];

  function visit(node) {
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      ts.isIdentifier(node.tagName) &&
      node.tagName.text === "button"
    ) {
      rawButtons.push(indexToPosition(sourceFile, node.getStart(sourceFile)));
    }

    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "buttonVariants"
    ) {
      buttonVariantsCalls.push(indexToPosition(sourceFile, node.expression.getStart(sourceFile)));
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);

  return {
    buttonVariantsCalls,
    rawButtons,
  };
}

function buildAllowlistMap(allowlist = approvedButtonUsageAllowlist) {
  return new Map(allowlist.map((entry) => [entry.relativeFilePath, entry]));
}

function formatPositions(positions) {
  return positions.map((position) => `${position.line}:${position.column}`).join(", ");
}

function getUsageViolation({
  actualCount,
  filePath,
  kind,
  positions,
  reason,
  recommendedFix,
  relativeFilePath,
}) {
  return {
    actualCount,
    filePath,
    kind,
    positions,
    reason,
    recommendedFix,
    relativeFilePath,
  };
}

export async function scanButtonUsage(
  rootDirectory = sourceDir,
  allowlist = approvedButtonUsageAllowlist,
) {
  const sourceFiles = await collectSourceFiles(rootDirectory);
  const rootUiDirectory = path.dirname(rootDirectory);
  const allowlistMap = buildAllowlistMap(allowlist);
  const observedAllowlistEntries = new Set();
  const staleAllowlistEntries = [];
  const violations = [];

  for (const sourceFile of sourceFiles.sort()) {
    const sourceText = await readFile(sourceFile, "utf8");
    const usage = collectButtonUsage(sourceText, sourceFile);
    const relativeFilePath = toUiRelativePath(sourceFile, rootUiDirectory);
    const allowlistEntry = allowlistMap.get(relativeFilePath);

    if (
      allowlistEntry &&
      (usage.rawButtons.length > 0 || usage.buttonVariantsCalls.length > 0)
    ) {
      observedAllowlistEntries.add(relativeFilePath);
    }

    const allowedRawButtonCount = allowlistEntry?.rawButtonCount ?? 0;
    if (usage.rawButtons.length > allowedRawButtonCount) {
      violations.push(
        getUsageViolation({
          actualCount: usage.rawButtons.length,
          filePath: sourceFile,
          kind: "raw-button",
          positions: usage.rawButtons,
          reason:
            allowlistEntry?.rawButtonReason ??
            "Ordinary production actions must use Button, compact dashboard actions must use DashboardActionButton, and semantic-button exceptions must move behind a dedicated wrapper or an allowlisted narrow exception path.",
          recommendedFix:
            allowlistEntry?.rawButtonReason
              ? "Reduce the raw <button> count back to the approved narrow exception footprint or migrate the extra controls onto Button, DashboardActionButton, or a dedicated semantic wrapper."
              : "Migrate this control onto Button, DashboardActionButton, or a dedicated semantic wrapper before keeping raw <button> ownership in production ui/src.",
          relativeFilePath,
        }),
      );
    }

    const allowedButtonVariantsCount = allowlistEntry?.buttonVariantsCount ?? 0;
    if (usage.buttonVariantsCalls.length > allowedButtonVariantsCount) {
      violations.push(
        getUsageViolation({
          actualCount: usage.buttonVariantsCalls.length,
          filePath: sourceFile,
          kind: "button-variants",
          positions: usage.buttonVariantsCalls,
          reason:
            allowlistEntry?.buttonVariantsReason ??
            "Direct buttonVariants ownership is reserved for shared primitive owners only.",
          recommendedFix:
            allowlistEntry?.buttonVariantsReason
              ? "Reduce direct buttonVariants ownership back to the approved shared-owner footprint or project the shared Button primitive with asChild."
              : "Use the shared Button primitive instead of owning buttonVariants(...) directly in production ui/src.",
          relativeFilePath,
        }),
      );
    }
  }

  for (const entry of allowlist) {
    const sourceFilePath = path.join(rootUiDirectory, entry.relativeFilePath);
    const sourceText = await readFile(sourceFilePath, "utf8").catch(() => null);

    if (sourceText === null) {
      staleAllowlistEntries.push({
        reason: "Allowlist entry points at a file that no longer exists.",
        relativeFilePath: entry.relativeFilePath,
      });
      continue;
    }

    const usage = collectButtonUsage(sourceText, sourceFilePath);
    if (
      entry.rawButtonCount !== undefined &&
      usage.rawButtons.length < entry.rawButtonCount
    ) {
      staleAllowlistEntries.push({
        reason: `Allowlisted raw <button> count is stale. Expected ${entry.rawButtonCount}, found ${usage.rawButtons.length}.`,
        relativeFilePath: entry.relativeFilePath,
      });
    }

    if (
      entry.buttonVariantsCount !== undefined &&
      usage.buttonVariantsCalls.length < entry.buttonVariantsCount
    ) {
      staleAllowlistEntries.push({
        reason: `Allowlisted buttonVariants(...) count is stale. Expected ${entry.buttonVariantsCount}, found ${usage.buttonVariantsCalls.length}.`,
        relativeFilePath: entry.relativeFilePath,
      });
    }
  }

  return {
    staleAllowlistEntries,
    violations,
  };
}

function formatViolation(rootDirectory, violation) {
  return [
    `${path.relative(rootDirectory, violation.filePath)}:${formatPositions(violation.positions)}`,
    `  kind: ${violation.kind}`,
    `  observed count: ${violation.actualCount}`,
    `  allowed path: ${violation.reason}`,
    `  fix: ${violation.recommendedFix}`,
  ].join("\n");
}

async function main() {
  const report = await scanButtonUsage(sourceDir, getConfiguredAllowlist());

  if (report.violations.length === 0 && report.staleAllowlistEntries.length === 0) {
    return;
  }

  const violations = report.violations.map((violation) => formatViolation(uiDir, violation)).join("\n\n");
  const staleAllowlistEntries = report.staleAllowlistEntries
    .map((entry) => [entry.relativeFilePath, `  ${entry.reason}`].join("\n"))
    .join("\n\n");

  console.error(
    [
      "Button usage guard failed.",
      "Production ui/src code must keep ordinary actions on Button, compact dashboard actions on DashboardActionButton, and raw semantic-button ownership behind shared wrapper owners or explicitly allowlisted narrow exceptions.",
      violations ? ["Violations:", violations].join("\n\n") : "",
      staleAllowlistEntries
        ? ["Stale allowlist entries:", staleAllowlistEntries].join("\n\n")
        : "",
    ]
      .filter(Boolean)
      .join("\n\n"),
  );
  process.exitCode = 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
