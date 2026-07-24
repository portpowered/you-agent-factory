import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { resolveRelativeImport } from "./resolve-relative-import.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultPackageDir = path.resolve(scriptDir, "..");
const defaultPackageSrcDir = path.join(defaultPackageDir, "src");
const packageImportPrefix = "@you-agent-factory/components";
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx"]);
const skippedFileSuffixes = [
  ".test.js",
  ".test.jsx",
  ".test.ts",
  ".test.tsx",
  ".stories.tsx",
];
const infrastructureSourceFiles = new Set([
  "category-paths.ts",
  "index.ts",
  "vite-aliases.ts",
  "styles.css",
]);
const packageLayerRanks = {
  tokens: 0,
  styles: 0,
  utilities: 1,
  icons: 1,
  primitives: 2,
  forms: 3,
  layout: 3,
  feedback: 3,
  "data-display": 3,
  navigation: 3,
  overlays: 3,
  charts: 4,
  graphs: 4,
  recipes: 5,
};
const testingLayerName = "testing";

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

function getLayerRank(layerName) {
  if (layerName === testingLayerName) {
    return Object.values(packageLayerRanks).length;
  }

  return packageLayerRanks[layerName] ?? null;
}

function classifyPackageLayerForRelativePath(relativeToPackageSrc) {
  const [segment] = relativeToPackageSrc.split("/");

  if (segment === "styles") {
    return "styles";
  }

  if (segment === testingLayerName) {
    return testingLayerName;
  }

  if (Object.hasOwn(packageLayerRanks, segment)) {
    return segment;
  }

  return null;
}

function classifyPackageLayerForFile(filePath, packageSrcDir) {
  const relativeToPackageSrc = toPosixPath(
    path.relative(packageSrcDir, filePath),
  );

  if (infrastructureSourceFiles.has(relativeToPackageSrc)) {
    return "infrastructure";
  }

  return classifyPackageLayerForRelativePath(relativeToPackageSrc);
}

function classifyPackageImport(specifier) {
  if (
    specifier === packageImportPrefix ||
    specifier.startsWith(`${packageImportPrefix}/styles.css`)
  ) {
    return "infrastructure";
  }

  if (!specifier.startsWith(`${packageImportPrefix}/`)) {
    return null;
  }

  const remainder = specifier.slice(`${packageImportPrefix}/`.length);
  const [categoryPath] = remainder.split("/");

  if (categoryPath === testingLayerName) {
    return testingLayerName;
  }

  if (Object.hasOwn(packageLayerRanks, categoryPath)) {
    return categoryPath;
  }

  return null;
}

function classifyResolvedPackagePath(resolvedPath, packageSrcDir) {
  const relativeToPackageSrc = toPosixPath(
    path.relative(packageSrcDir, resolvedPath),
  );

  if (relativeToPackageSrc.startsWith("..")) {
    return null;
  }

  return classifyPackageLayerForRelativePath(relativeToPackageSrc);
}

function classifyImport(specifier, filePath, packageSrcDir) {
  const packageLayer = classifyPackageImport(specifier);
  if (packageLayer) {
    return {
      layer: packageLayer,
      resolvedPath: null,
    };
  }

  const resolvedPath = resolveRelativeImport(specifier, filePath);
  if (!resolvedPath) {
    return null;
  }

  const layer = classifyResolvedPackagePath(resolvedPath, packageSrcDir);
  if (!layer) {
    return null;
  }

  return {
    layer,
    resolvedPath,
  };
}

function classifyDependencyViolation(sourceLayer, targetLayer) {
  if (sourceLayer === "infrastructure" || targetLayer === "infrastructure") {
    return null;
  }

  if (sourceLayer !== testingLayerName && targetLayer === testingLayerName) {
    return {
      kind: "testing-support-import",
      message:
        "Package production source must not import testing support modules.",
    };
  }

  const sourceRank = getLayerRank(sourceLayer);
  const targetRank = getLayerRank(targetLayer);

  if (sourceRank === null || targetRank === null) {
    return null;
  }

  if (targetRank > sourceRank) {
    return {
      kind: "package-layer-violation",
      message: `Package layer "${sourceLayer}" must not import higher layer "${targetLayer}".`,
    };
  }

  return null;
}

function collectImportViolations(sourceText, filePath, packageSrcDir) {
  const sourceLayer = classifyPackageLayerForFile(filePath, packageSrcDir);
  if (sourceLayer === "infrastructure") {
    return [];
  }

  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const violations = [];

  function recordImport(specifier) {
    const classification = classifyImport(specifier, filePath, packageSrcDir);
    if (!classification) {
      return;
    }

    const violation = classifyDependencyViolation(
      sourceLayer,
      classification.layer,
    );
    if (!violation) {
      return;
    }

    violations.push({
      importPath: specifier,
      kind: violation.kind,
      message: violation.message,
      resolvedPath: classification.resolvedPath ?? null,
      sourceLayer,
      targetLayer: classification.layer,
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
    `  source layer: ${violation.sourceLayer}`,
    `  target layer: ${violation.targetLayer}`,
    `  reason: ${violation.message}`,
  ].join("\n");
}

function resolveConfiguredDirectories() {
  return {
    packageSrcDir: process.env.AGENT_FACTORY_COMPONENTS_SRC_DIR
      ? path.resolve(process.env.AGENT_FACTORY_COMPONENTS_SRC_DIR)
      : defaultPackageSrcDir,
  };
}

export async function scanPackageDependencyDirection(
  packageSrcDir = defaultPackageSrcDir,
) {
  const packageDir = path.dirname(packageSrcDir);
  const files = await collectSourceFiles(packageSrcDir);
  const violations = [];

  for (const filePath of files.sort()) {
    const sourceText = await readFile(filePath, "utf8");
    const importViolations = collectImportViolations(
      sourceText,
      filePath,
      packageSrcDir,
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
  const { packageSrcDir } = resolveConfiguredDirectories();
  const report = await scanPackageDependencyDirection(packageSrcDir);

  if (report.violations.length === 0) {
    process.stdout.write(
      "@you-agent-factory/components package dependency-direction check passed.\n",
    );
    return;
  }

  process.stderr.write(
    "@you-agent-factory/components package dependency-direction check failed:\n",
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
