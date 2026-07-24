import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { resolveRelativeImport } from "./resolve-relative-import.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultPackageDir = path.resolve(scriptDir, "..");
const defaultPackageSrcDir = path.join(defaultPackageDir, "src");
const defaultDashboardSrcDir = path.resolve(
  defaultPackageDir,
  "..",
  "..",
  "src",
);
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx"]);
const skippedFileSuffixes = [
  ".test.js",
  ".test.jsx",
  ".test.ts",
  ".test.tsx",
  ".stories.tsx",
];
const forbiddenRuntimeModules = new Set(["zustand"]);
const forbiddenRuntimeModulePrefixes = [
  "@tanstack/react-query",
  "monaco-editor",
  "@monaco-editor/react",
  "react-grid-layout",
  "sonner",
];

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
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

function classifyRuntimeModuleImport(specifier) {
  if (forbiddenRuntimeModules.has(specifier)) {
    return {
      kind: "app-runtime-module",
      message:
        "Package source must not import app-only runtime modules such as Zustand.",
    };
  }

  const forbiddenPrefix = forbiddenRuntimeModulePrefixes.find(
    (modulePrefix) =>
      specifier === modulePrefix || specifier.startsWith(`${modulePrefix}/`),
  );
  if (!forbiddenPrefix) {
    return null;
  }

  return {
    kind: "app-runtime-module",
    message:
      "Package source must not import app-only runtime modules such as React Query, Monaco, or Sonner.",
  };
}

function classifyDashboardSrcImport(relativeToDashboardSrc) {
  if (relativeToDashboardSrc.startsWith("api/generated/")) {
    return {
      kind: "generated-client-import",
      message:
        "Package source must not import generated OpenAPI clients from the dashboard app.",
    };
  }

  if (relativeToDashboardSrc.startsWith("api/")) {
    return {
      kind: "dashboard-api-import",
      message: "Package source must not import dashboard app API modules.",
    };
  }

  if (relativeToDashboardSrc.startsWith("features/dashboard/session/")) {
    return {
      kind: "dashboard-session-provider-import",
      message: "Package source must not import dashboard session providers.",
    };
  }

  if (relativeToDashboardSrc.startsWith("features/")) {
    return {
      kind: "dashboard-feature-import",
      message: "Package source must not import dashboard feature modules.",
    };
  }

  if (relativeToDashboardSrc.startsWith("i18n/")) {
    return {
      kind: "dashboard-i18n-import",
      message:
        "Package source must not import dashboard i18n providers or app locale modules.",
    };
  }

  return null;
}

function classifyImport(specifier, filePath, dashboardSrcDir) {
  const runtimeViolation = classifyRuntimeModuleImport(specifier);
  if (runtimeViolation) {
    return runtimeViolation;
  }

  const resolvedPath = resolveRelativeImport(specifier, filePath);
  if (!resolvedPath) {
    return null;
  }

  const relativeToDashboardSrc = toPosixPath(
    path.relative(dashboardSrcDir, resolvedPath),
  );
  if (relativeToDashboardSrc.startsWith("..")) {
    return null;
  }

  const dashboardViolation = classifyDashboardSrcImport(relativeToDashboardSrc);
  if (!dashboardViolation) {
    return null;
  }

  return {
    ...dashboardViolation,
    resolvedPath,
  };
}

function collectImportViolations(sourceText, filePath, dashboardSrcDir) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const violations = [];

  function recordImport(specifier) {
    const classification = classifyImport(specifier, filePath, dashboardSrcDir);
    if (!classification) {
      return;
    }

    violations.push({
      importPath: specifier,
      kind: classification.kind,
      message: classification.message,
      resolvedPath: classification.resolvedPath ?? null,
    });
  }

  function visit(node) {
    if (
      ts.isImportDeclaration(node) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      recordImport(node.moduleSpecifier.text);
    }

    if (
      ts.isExportDeclaration(node) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      recordImport(node.moduleSpecifier.text);
    }

    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments.length === 1 &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      recordImport(node.arguments[0].text);
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);

  return violations;
}

function formatViolation(packageDir, filePath, violation) {
  const relativeFilePath = toPosixPath(path.relative(packageDir, filePath));
  const resolvedSuffix = violation.resolvedPath
    ? ` -> ${toPosixPath(path.relative(packageDir, violation.resolvedPath))}`
    : "";

  return [
    `- ${relativeFilePath} imports ${violation.importPath}${resolvedSuffix}`,
    `  kind: ${violation.kind}`,
    `  reason: ${violation.message}`,
  ].join("\n");
}

function resolveConfiguredDirectories() {
  return {
    dashboardSrcDir: process.env.AGENT_FACTORY_DASHBOARD_SRC_DIR
      ? path.resolve(process.env.AGENT_FACTORY_DASHBOARD_SRC_DIR)
      : defaultDashboardSrcDir,
    packageSrcDir: process.env.AGENT_FACTORY_COMPONENTS_SRC_DIR
      ? path.resolve(process.env.AGENT_FACTORY_COMPONENTS_SRC_DIR)
      : defaultPackageSrcDir,
  };
}

export async function scanPackageBoundary(
  packageSrcDir = defaultPackageSrcDir,
  dashboardSrcDir = defaultDashboardSrcDir,
) {
  const packageDir = path.dirname(packageSrcDir);
  const files = await collectSourceFiles(packageSrcDir);
  const violations = [];

  for (const filePath of files.sort()) {
    const sourceText = await readFile(filePath, "utf8");
    const importViolations = collectImportViolations(
      sourceText,
      filePath,
      dashboardSrcDir,
    );

    for (const importViolation of importViolations) {
      violations.push({
        ...importViolation,
        filePath,
        relativeFilePath: toPosixPath(path.relative(packageDir, filePath)),
      });
    }
  }

  return {
    packageDir,
    violations,
  };
}

async function main() {
  const { dashboardSrcDir, packageSrcDir } = resolveConfiguredDirectories();
  const report = await scanPackageBoundary(packageSrcDir, dashboardSrcDir);

  if (report.violations.length === 0) {
    process.stdout.write(
      "@you-agent-factory/components package boundary check passed.\n",
    );
    return;
  }

  process.stderr.write(
    "@you-agent-factory/components package boundary check failed:\n",
  );

  for (const violation of report.violations) {
    process.stderr.write(
      `${formatViolation(report.packageDir, violation.filePath, violation)}\n`,
    );
  }

  process.exitCode = 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
