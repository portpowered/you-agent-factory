import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const packageDir = path.resolve(scriptDir, "..");
const packageSrcDir = path.join(packageDir, "src");
const dashboardSrcDir = path.resolve(packageDir, "..", "..", "src");
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx"]);
const skippedFileSuffixes = [
  ".test.js",
  ".test.jsx",
  ".test.ts",
  ".test.tsx",
  ".stories.tsx",
];
const forbiddenDashboardImportPrefixes = [
  `${dashboardSrcDir}/api/`,
  `${dashboardSrcDir}/features/`,
  `${dashboardSrcDir}/api/generated/`,
];

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

function resolveImportPath(importPath, fromFile) {
  if (!importPath.startsWith(".")) {
    return null;
  }

  const resolved = path.resolve(path.dirname(fromFile), importPath);
  const extensions = [".ts", ".tsx", ".js", ".jsx", "/index.ts", "/index.tsx"];

  for (const extension of extensions) {
    const candidate = resolved.endsWith(extension)
      ? resolved
      : `${resolved}${extension}`;
    if (candidate.startsWith(dashboardSrcDir)) {
      return candidate;
    }
  }

  return resolved.startsWith(dashboardSrcDir) ? resolved : null;
}

function collectImportViolations(filePath, sourceText) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const violations = [];

  for (const statement of sourceFile.statements) {
    if (!ts.isImportDeclaration(statement) || !statement.moduleSpecifier) {
      continue;
    }

    if (!ts.isStringLiteral(statement.moduleSpecifier)) {
      continue;
    }

    const importPath = statement.moduleSpecifier.text;
    const resolvedPath = resolveImportPath(importPath, filePath);
    if (!resolvedPath) {
      continue;
    }

    for (const forbiddenPrefix of forbiddenDashboardImportPrefixes) {
      if (resolvedPath.startsWith(forbiddenPrefix)) {
        violations.push({
          filePath,
          importPath,
          resolvedPath,
        });
      }
    }
  }

  return violations;
}

async function main() {
  const files = await collectSourceFiles(packageSrcDir);
  const violations = [];

  for (const filePath of files) {
    const sourceText = await readFile(filePath, "utf8");
    violations.push(...collectImportViolations(filePath, sourceText));
  }

  if (violations.length === 0) {
    process.stdout.write(
      "@you-agent-factory/components package boundary check passed.\n",
    );
    return;
  }

  process.stderr.write(
    "@you-agent-factory/components package boundary check failed:\n",
  );

  for (const violation of violations) {
    process.stderr.write(
      `- ${path.relative(packageDir, violation.filePath)} imports ${violation.importPath} -> ${path.relative(packageDir, violation.resolvedPath)}\n`,
    );
  }

  process.exitCode = 1;
}

await main();
