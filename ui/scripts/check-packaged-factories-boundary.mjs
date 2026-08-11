import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import ts from "typescript";

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const defaultUiRoot = path.resolve(scriptDirectory, "..");

export const prohibitedPackage = "@you-agent-factory/packaged-factories";

const sourceExtensions = new Set([".ts", ".tsx", ".js", ".jsx"]);
const manifestDependencySections = [
  "dependencies",
  "devDependencies",
  "optionalDependencies",
  "peerDependencies",
  "bundledDependencies",
  "bundleDependencies",
];

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
}

function isProhibitedSpecifier(specifier) {
  return (
    specifier === prohibitedPackage ||
    specifier.startsWith(`${prohibitedPackage}/`)
  );
}

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries.sort((left, right) =>
    left.name.localeCompare(right.name),
  )) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await sourceFiles(target)));
    } else if (sourceExtensions.has(path.extname(entry.name))) {
      files.push(target);
    }
  }
  return files;
}

function dependencyNames(value) {
  if (Array.isArray(value)) {
    return value.filter((dependency) => typeof dependency === "string");
  }
  if (value !== null && typeof value === "object") {
    return Object.keys(value);
  }
  return [];
}

function manifestViolations(manifest) {
  const violations = [];

  for (const section of manifestDependencySections) {
    if (!Object.hasOwn(manifest, section)) {
      continue;
    }

    if (dependencyNames(manifest[section]).includes(prohibitedPackage)) {
      violations.push({
        kind: "manifest-dependency",
        filePath: "package.json",
        section,
        message: `package.json [${section}] declares prohibited package ${prohibitedPackage}`,
      });
    }
  }

  return violations;
}

function scriptKind(filePath) {
  const extension = path.extname(filePath);
  if (extension === ".tsx") return ts.ScriptKind.TSX;
  if (extension === ".jsx") return ts.ScriptKind.JSX;
  if (extension === ".js") return ts.ScriptKind.JS;
  return ts.ScriptKind.TS;
}

function stringLiteralText(node) {
  return node && ts.isStringLiteralLike(node) ? node.text : null;
}

function sourceImportViolations(sourceText, filePath, uiRoot) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    scriptKind(filePath),
  );
  const relativeFilePath = toPosixPath(path.relative(uiRoot, filePath));
  const violations = [];

  function addViolation(node, specifier, syntax) {
    if (!specifier || !isProhibitedSpecifier(specifier)) {
      return;
    }

    const position = sourceFile.getLineAndCharacterOfPosition(
      node.getStart(sourceFile),
    );
    violations.push({
      kind: "source-import",
      filePath: relativeFilePath,
      line: position.line + 1,
      column: position.character + 1,
      specifier,
      syntax,
      message: `${relativeFilePath}:${position.line + 1}:${position.character + 1} uses ${syntax} to import prohibited package ${specifier}`,
    });
  }

  function visit(node) {
    if (ts.isImportDeclaration(node)) {
      addViolation(
        node,
        stringLiteralText(node.moduleSpecifier),
        "static import",
      );
    } else if (ts.isExportDeclaration(node)) {
      addViolation(
        node,
        stringLiteralText(node.moduleSpecifier),
        "module re-export",
      );
    } else if (ts.isImportEqualsDeclaration(node)) {
      const externalModule = ts.isExternalModuleReference(node.moduleReference)
        ? stringLiteralText(node.moduleReference.expression)
        : null;
      addViolation(node, externalModule, "static import");
    } else if (ts.isCallExpression(node)) {
      const specifier = stringLiteralText(node.arguments[0]);
      if (node.expression.kind === ts.SyntaxKind.ImportKeyword) {
        addViolation(node, specifier, "dynamic import");
      } else if (
        ts.isIdentifier(node.expression) &&
        node.expression.text === "require"
      ) {
        addViolation(node, specifier, "CommonJS import");
      }
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);
  return violations;
}

export async function inspectPackagedFactoriesBoundary({
  uiRoot = defaultUiRoot,
  sourceRoot = path.join(uiRoot, "src"),
} = {}) {
  const resolvedUiRoot = path.resolve(uiRoot);
  const resolvedSourceRoot = path.resolve(sourceRoot);
  const manifest = JSON.parse(
    await readFile(path.join(resolvedUiRoot, "package.json"), "utf8"),
  );
  const violations = manifestViolations(manifest);

  for (const filePath of await sourceFiles(resolvedSourceRoot)) {
    const sourceText = await readFile(filePath, "utf8");
    violations.push(
      ...sourceImportViolations(sourceText, filePath, resolvedUiRoot),
    );
  }

  return {
    sourceRoot: resolvedSourceRoot,
    uiRoot: resolvedUiRoot,
    violations,
  };
}

async function main({ uiRoot = defaultUiRoot, sourceRoot } = {}) {
  const report = await inspectPackagedFactoriesBoundary({
    uiRoot,
    sourceRoot: sourceRoot ?? path.join(uiRoot, "src"),
  });

  if (report.violations.length > 0) {
    process.stderr.write("Packaged Factories dashboard boundary failed:\n");
    for (const violation of report.violations) {
      process.stderr.write(`- [${violation.kind}] ${violation.message}\n`);
    }
    process.exitCode = 1;
    return;
  }

  process.stdout.write("Packaged Factories dashboard boundary passed.\n");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const configuredUiRoot = process.env.AGENT_FACTORY_PACKAGED_FACTORIES_UI_ROOT;
  const configuredSourceRoot =
    process.env.AGENT_FACTORY_PACKAGED_FACTORIES_SOURCE_ROOT;
  await main({
    uiRoot: configuredUiRoot ? path.resolve(configuredUiRoot) : defaultUiRoot,
    sourceRoot: configuredSourceRoot
      ? path.resolve(configuredSourceRoot)
      : undefined,
  });
}
