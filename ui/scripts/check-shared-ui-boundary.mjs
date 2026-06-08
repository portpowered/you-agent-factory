import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { allowlistedSharedUiBoundaryViolations } from "./shared-ui-boundary-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const defaultSrcDir = path.join(defaultUiDir, "src");
const sharedUiDir = process.env.AGENT_FACTORY_UI_SHARED_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_SHARED_DIR)
  : path.join(defaultSrcDir, "components", "ui");
const srcDir = path.dirname(path.dirname(sharedUiDir));
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
  process.env.AGENT_FACTORY_UI_SHARED_UI_BOUNDARY_ALLOWLIST;
const featureStateRuntimeModules = new Set(["zustand"]);
const featureNetworkRuntimeModulePrefixes = ["@tanstack/react-query"];

function getConfiguredAllowlist() {
  if (!allowlistOverride) {
    return allowlistedSharedUiBoundaryViolations;
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

  for (const candidateExtension of [".tsx", ".ts", ".jsx", ".js"]) {
    const candidatePath = `${resolvedPath}${candidateExtension}`;
    return candidatePath;
  }

  return resolvedPath;
}

function classifyImport(specifier, filePath, rootSrcDir) {
  const runtimeStateModule =
    featureStateRuntimeModules.has(specifier) ||
    [...featureStateRuntimeModules].some((moduleName) =>
      specifier.startsWith(`${moduleName}/`),
    );
  if (runtimeStateModule) {
    return {
      kind: "feature-state-runtime",
      message:
        "Shared UI production files must not own feature state containers. Keep Zustand stores and selectors under ui/src/features/<feature>/state/.",
    };
  }

  const runtimeNetworkModule = featureNetworkRuntimeModulePrefixes.some(
    (modulePrefix) =>
      specifier === modulePrefix || specifier.startsWith(`${modulePrefix}/`),
  );
  if (runtimeNetworkModule) {
    return {
      kind: "feature-network-runtime",
      message:
        "Shared UI production files must not own feature network orchestration. Keep React Query hooks and API workflows under ui/src/features/<feature>/hooks/ or lib/.",
    };
  }

  const resolvedPath = resolveRelativeImport(specifier, filePath);
  if (!resolvedPath) {
    return null;
  }

  const relativeToSrc = toPosixPath(path.relative(rootSrcDir, resolvedPath));
  if (!relativeToSrc.startsWith("features/")) {
    return null;
  }

  if (relativeToSrc.includes("/state/")) {
    return {
      kind: "feature-state-import",
      message:
        "Shared UI production files must not import feature-owned state containers from ui/src/features/<feature>/state/.",
    };
  }

  if (relativeToSrc.includes("/hooks/")) {
    return {
      kind: "feature-network-import",
      message:
        "Shared UI production files must not import feature-owned network modules from ui/src/features/<feature>/hooks/.",
    };
  }

  return {
    kind: "feature-import",
    message:
      "Shared UI production files must not import feature-owned modules from ui/src/features/. Keep feature workflows in feature layers and expose only shared styling primitives or shared presentation wrappers from ui/src/components/ui.",
  };
}

function collectImportViolations(sourceText, filePath, rootSrcDir) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const violations = [];

  function recordImport(specifier, position) {
    const classification = classifyImport(specifier, filePath, rootSrcDir);
    if (!classification) {
      return;
    }

    violations.push({
      kind: classification.kind,
      message: classification.message,
      position,
      specifier,
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
    `  kind: ${violation.kind}`,
    `  reason: ${violation.message}`,
    "  fix: ui/src/components/ui is reserved for shared styling primitives and shared presentation owners. Move feature-owned behavior into ui/src/features/<feature>/ and consume shared primitives from the feature layer.",
  ].join("\n");
}

export async function scanSharedUiBoundary(
  rootDirectory = sharedUiDir,
  allowlist = allowlistedSharedUiBoundaryViolations,
) {
  const rootSrcDir = path.dirname(path.dirname(rootDirectory));
  const rootUiDir = path.dirname(rootSrcDir);
  const sourceFiles = await collectSourceFiles(rootDirectory);
  const allowlistMap = buildAllowlistMap(allowlist);
  const observedAllowlistKeys = new Set();
  const allowlistedDebt = [];
  const violations = [];

  for (const sourceFile of sourceFiles.sort()) {
    const sourceText = await readFile(sourceFile, "utf8");
    const relativeFilePath = toUiRelativePath(sourceFile, rootUiDir);
    const importViolations = collectImportViolations(
      sourceText,
      sourceFile,
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
      });
    }
  }

  const staleAllowlistEntries = [...allowlistMap.entries()]
    .filter(([allowlistKey]) => !observedAllowlistKeys.has(allowlistKey))
    .map(([, allowlistEntry]) => ({
      allowlistEntry,
      reason:
        "Allowlist entry is stale. Remove it in the same change that deleted or relocated the allowlisted shared-ui boundary violation.",
    }));

  return {
    allowlistedDebt,
    staleAllowlistEntries,
    violations,
  };
}

async function main() {
  const report = await scanSharedUiBoundary(
    sharedUiDir,
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

  if (report.violations.length > 0 || report.staleAllowlistEntries.length > 0) {
    const violationReport = report.violations
      .map((entry) => formatViolation(entry.relativeFilePath, entry))
      .join("\n\n");

    console.error(
      [
        "Shared UI boundary guard failed.",
        "ui/src/components/ui is reserved for shared styling primitives and shared presentation owners.",
        "Production files in this lane must not import feature-owned modules, state containers, or network workflows from ui/src/features.",
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

  if (allowlistedDebtReport.length > 0) {
    console.log(
      [
        "Shared UI boundary guard passed with allowlisted legacy debt.",
        "Current allowlisted shared-ui boundary violations:",
        allowlistedDebtReport,
      ].join("\n\n"),
    );
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
