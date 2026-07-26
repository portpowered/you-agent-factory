import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { allowlistedCrossFeatureBoundaryViolations } from "./feature-cross-import-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const defaultSrcDir = path.join(defaultUiDir, "src");
const featuresDir = process.env.AGENT_FACTORY_UI_FEATURES_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_FEATURES_DIR)
  : path.join(defaultSrcDir, "features");
const srcDir = path.dirname(featuresDir);
const uiDir = path.dirname(srcDir);
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx"]);
const skippedFileSuffixes = [
  ".test.js",
  ".test.jsx",
  ".test.ts",
  ".test.tsx",
  ".stories.tsx",
];
const allowlistOverride =
  process.env.AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_ALLOWLIST;
const strictBoundaryMode =
  process.env.AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_STRICT === "1";

function getConfiguredAllowlist() {
  if (!allowlistOverride) {
    return allowlistedCrossFeatureBoundaryViolations;
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

  if (toPosixPath(filePath).includes("/test-support/")) {
    return true;
  }

  return skippedFileSuffixes.some((suffix) => filePath.endsWith(suffix));
}

async function collectSourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
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

function resolveRelativeImport(specifier, filePath) {
  if (!specifier.startsWith(".")) {
    return null;
  }

  const resolvedPath = path.resolve(path.dirname(filePath), specifier);
  const extension = path.extname(resolvedPath);

  if (extension.length > 0) {
    return resolvedPath;
  }

  return resolvedPath;
}

function getFeatureName(relativeToSrc) {
  const match = relativeToSrc.match(/^features\/([^/]+)\//);
  return match ? match[1] : null;
}

function isPublicBoundary(relativeToSrc) {
  return /\/public(?:\/|$)/.test(relativeToSrc);
}

function classifyCrossFeatureImport(
  specifier,
  filePath,
  sourceFeatureName,
  rootSrcDir,
) {
  const resolvedPath = resolveRelativeImport(specifier, filePath);
  if (!resolvedPath) {
    return null;
  }

  const relativeToSrc = toPosixPath(path.relative(rootSrcDir, resolvedPath));
  const targetFeatureName = getFeatureName(relativeToSrc);
  if (!targetFeatureName || targetFeatureName === sourceFeatureName) {
    return null;
  }

  if (isPublicBoundary(relativeToSrc)) {
    return null;
  }

  return {
    message: `Cross-feature imports must use the target feature public/ boundary. Import from ui/src/features/${targetFeatureName}/public/ instead of ${relativeToSrc}.`,
    targetFeatureName,
    targetRelativePath: relativeToSrc,
  };
}

function collectCrossFeatureImportViolations(
  sourceText,
  filePath,
  sourceFeatureName,
  rootSrcDir,
) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const violations = [];

  function recordImport(specifier, position) {
    const classification = classifyCrossFeatureImport(
      specifier,
      filePath,
      sourceFeatureName,
      rootSrcDir,
    );
    if (!classification) {
      return;
    }

    violations.push({
      message: classification.message,
      position,
      specifier,
      targetFeatureName: classification.targetFeatureName,
      targetRelativePath: classification.targetRelativePath,
    });
  }

  function visit(node) {
    if (
      ts.isImportDeclaration(node) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      recordImport(
        node.moduleSpecifier.text,
        indexToPosition(sourceFile, node.moduleSpecifier.getStart(sourceFile)),
      );
    }

    if (
      ts.isExportDeclaration(node) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      recordImport(
        node.moduleSpecifier.text,
        indexToPosition(sourceFile, node.moduleSpecifier.getStart(sourceFile)),
      );
    }

    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments.length === 1 &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      recordImport(
        node.arguments[0].text,
        indexToPosition(sourceFile, node.arguments[0].getStart(sourceFile)),
      );
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);

  return violations;
}

function createAllowlistKey(relativeFilePath, specifier) {
  return `${relativeFilePath}#${specifier}`;
}

function buildAllowlistMap(allowlist) {
  const allowlistMap = new Map();

  for (const entry of allowlist) {
    for (const specifier of entry.importSpecifiers) {
      allowlistMap.set(createAllowlistKey(entry.relativeFilePath, specifier), {
        ...entry,
        specifier,
      });
    }
  }

  return allowlistMap;
}

function formatViolation(relativeFilePath, violation) {
  return [
    `${relativeFilePath}:${violation.position.line}:${violation.position.column}`,
    `  import: ${violation.specifier}`,
    `  target: ${violation.targetRelativePath}`,
    `  reason: ${violation.message}`,
    "  fix: Route cross-feature reuse through ui/src/features/<feature>/public/ or relocate shared behavior into an approved owner.",
  ].join("\n");
}

export async function scanCrossFeatureBoundary(
  rootDirectory = featuresDir,
  allowlist = allowlistedCrossFeatureBoundaryViolations,
) {
  const rootSrcDir = path.dirname(rootDirectory);
  const rootUiDir = path.dirname(rootSrcDir);
  const sourceFiles = await collectSourceFiles(rootDirectory);
  const allowlistMap = buildAllowlistMap(allowlist);
  const observedAllowlistKeys = new Set();
  const allowlistedDebt = [];
  const violations = [];

  for (const sourceFile of sourceFiles.sort()) {
    const sourceText = await readFile(sourceFile, "utf8");
    const relativeFilePath = toUiRelativePath(sourceFile, rootUiDir);
    const sourceFeatureName = getFeatureName(
      toPosixPath(path.relative(rootSrcDir, sourceFile)),
    );
    if (!sourceFeatureName) {
      continue;
    }

    const importViolations = collectCrossFeatureImportViolations(
      sourceText,
      sourceFile,
      sourceFeatureName,
      rootSrcDir,
    );

    for (const importViolation of importViolations) {
      const allowlistKey = createAllowlistKey(
        relativeFilePath,
        importViolation.specifier,
      );
      const allowlistEntry = allowlistMap.get(allowlistKey);

      if (allowlistEntry) {
        observedAllowlistKeys.add(allowlistKey);
        allowlistedDebt.push({
          allowlistEntry,
          importViolation,
          relativeFilePath,
        });
        continue;
      }

      violations.push({
        ...importViolation,
        filePath: sourceFile,
        relativeFilePath,
        sourceFeatureName,
      });
    }
  }

  const staleAllowlistEntries = [...allowlistMap.entries()]
    .filter(([allowlistKey]) => !observedAllowlistKeys.has(allowlistKey))
    .map(([, allowlistEntry]) => ({
      allowlistEntry,
      reason:
        "Allowlist entry is stale. Remove it in the same change that deleted or rerouted the allowlisted cross-feature import.",
    }));

  return {
    allowlistedDebt,
    staleAllowlistEntries,
    violations,
  };
}

async function main() {
  const report = await scanCrossFeatureBoundary(
    featuresDir,
    getConfiguredAllowlist(),
  );
  const allowlistedDebtReport = report.allowlistedDebt
    .map((entry) =>
      formatViolation(entry.relativeFilePath, entry.importViolation),
    )
    .join("\n\n");
  const staleAllowlistReport = report.staleAllowlistEntries
    .map((entry) =>
      [
        `${entry.allowlistEntry.relativeFilePath}#${entry.allowlistEntry.specifier}`,
        `  ${entry.reason}`,
      ].join("\n"),
    )
    .join("\n\n");

  if (
    strictBoundaryMode &&
    (report.violations.length > 0 || report.staleAllowlistEntries.length > 0)
  ) {
    const violationReport = report.violations
      .map((entry) => formatViolation(entry.relativeFilePath, entry))
      .join("\n\n");

    console.error(
      [
        "Feature boundary guard failed.",
        "Cross-feature imports must use intentional public/ boundaries instead of reaching into another feature's internals.",
        violationReport
          ? ["New hard-fail violations:", violationReport].join("\n\n")
          : "",
        staleAllowlistReport
          ? ["Stale allowlist entries:", staleAllowlistReport].join("\n\n")
          : "",
        allowlistedDebtReport
          ? ["Allowlisted legacy debt:", allowlistedDebtReport].join("\n\n")
          : "",
      ]
        .filter(Boolean)
        .join("\n\n"),
    );
    process.exitCode = 1;
    return;
  }

  if (report.violations.length > 0 || report.staleAllowlistEntries.length > 0) {
    console.log(
      [
        "Feature boundary advisory passed.",
        `${report.violations.length} focused cross-feature import(s) and ${report.staleAllowlistEntries.length} stale allowlist entry/entries were observed.`,
        "Focused direct imports are permitted when they avoid aggregate module fan-out; aggregate public barrels remain prohibited by check:test-lanes.",
        "Set AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_STRICT=1 to audit the legacy strict policy.",
      ].join("\n"),
    );
    return;
  }

  if (allowlistedDebtReport.length > 0) {
    console.log(
      [
        "Feature boundary guard passed with allowlisted legacy debt.",
        "Current allowlisted cross-feature boundary violations:",
        allowlistedDebtReport,
      ].join("\n\n"),
    );
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
