import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { allowlistedInlineComponentClassUsage } from "./inline-component-class-usage-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const sourceDir = process.env.AGENT_FACTORY_UI_SRC_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_SRC_DIR)
  : path.join(defaultUiDir, "src");
const uiDir = path.dirname(sourceDir);
const skippedFileSuffixes = [".test.tsx", ".stories.tsx"];
const skippedDirectoryNames = new Set(["generated"]);
const skippedPathFragments = [`${path.sep}api${path.sep}generated${path.sep}`];
const allowlistOverride = process.env.AGENT_FACTORY_UI_INLINE_CLASS_ALLOWLIST;
const classConstantNamePattern = /(?:^|_)(?:CLASS|CLASS_NAME)$|Class(?:Name)?$/;

function getConfiguredAllowlist() {
  if (!allowlistOverride) {
    return allowlistedInlineComponentClassUsage;
  }

  return allowlistOverride
    .split(/\r?\n/)
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
}

function toUiRelativePath(filePath, rootUiDirectory = uiDir) {
  return toPosixPath(path.relative(rootUiDirectory, filePath));
}

function createAllowlistKey(relativeFilePath, constantName) {
  return `${relativeFilePath}#${constantName}`;
}

function shouldSkipFile(filePath) {
  if (!filePath.endsWith(".tsx")) {
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

function indexToPosition(sourceText, index) {
  let line = 1;
  let column = 1;

  for (let cursor = 0; cursor < index; cursor += 1) {
    if (sourceText[cursor] === "\n") {
      line += 1;
      column = 1;
      continue;
    }

    column += 1;
  }

  return { column, line };
}

function hasExportModifier(node) {
  return node.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword) ?? false;
}

function isClassConstantCandidate(statement, declaration) {
  if (!ts.isVariableStatement(statement)) {
    return false;
  }

  if ((statement.declarationList.flags & ts.NodeFlags.Const) === 0) {
    return false;
  }

  if (hasExportModifier(statement)) {
    return false;
  }

  return (
    ts.isIdentifier(declaration.name) &&
    declaration.initializer !== undefined &&
    classConstantNamePattern.test(declaration.name.text)
  );
}

function getIdentifierText(expression) {
  if (ts.isIdentifier(expression)) {
    return expression.text;
  }

  return null;
}

function collectImportedBindings(sourceFile) {
  const importedBindings = new Set();

  for (const statement of sourceFile.statements) {
    if (!ts.isImportDeclaration(statement) || statement.importClause === undefined) {
      continue;
    }

    if (statement.importClause.name) {
      importedBindings.add(statement.importClause.name.text);
    }

    const { namedBindings } = statement.importClause;
    if (!namedBindings) {
      continue;
    }

    if (ts.isNamespaceImport(namedBindings)) {
      importedBindings.add(namedBindings.name.text);
      continue;
    }

    for (const element of namedBindings.elements) {
      importedBindings.add(element.name.text);
    }
  }

  return importedBindings;
}

function collectTopLevelDeclarations(sourceFile) {
  const declarations = new Map();

  for (const statement of sourceFile.statements) {
    if (!ts.isVariableStatement(statement)) {
      continue;
    }

    for (const declaration of statement.declarationList.declarations) {
      if (ts.isIdentifier(declaration.name)) {
        declarations.set(declaration.name.text, { declaration, statement });
      }
    }
  }

  return declarations;
}

function isStaticClassExpression(expression, declarations, importedBindings, visitedNames = new Set()) {
  if (ts.isParenthesizedExpression(expression)) {
    return isStaticClassExpression(expression.expression, declarations, importedBindings, visitedNames);
  }

  if (ts.isStringLiteral(expression) || ts.isNoSubstitutionTemplateLiteral(expression)) {
    return true;
  }

  if (ts.isIdentifier(expression)) {
    if (importedBindings.has(expression.text)) {
      return true;
    }

    if (visitedNames.has(expression.text)) {
      return false;
    }

    const record = declarations.get(expression.text);
    if (!record || record.declaration.initializer === undefined) {
      return false;
    }

    visitedNames.add(expression.text);
    const isStatic = isStaticClassExpression(
      record.declaration.initializer,
      declarations,
      importedBindings,
      visitedNames,
    );
    visitedNames.delete(expression.text);
    return isStatic;
  }

  if (!ts.isCallExpression(expression) || getIdentifierText(expression.expression) !== "cn") {
    return false;
  }

  return expression.arguments.every((argument) =>
    isStaticClassExpression(argument, declarations, importedBindings, visitedNames),
  );
}

function isDirectClassNameReference(identifier) {
  return (
    ts.isJsxExpression(identifier.parent) &&
    identifier.parent.expression === identifier &&
    ts.isJsxAttribute(identifier.parent.parent) &&
    identifier.parent.parent.name.text === "className"
  );
}

function collectReferences(sourceFile, constantName, declarationName, checker) {
  const references = [];
  const declarationSymbol = checker.getSymbolAtLocation(declarationName);

  if (!declarationSymbol) {
    return references;
  }

  const visit = (node) => {
    if (
      ts.isIdentifier(node) &&
      node.text === constantName &&
      node !== declarationName &&
      checker.getSymbolAtLocation(node) === declarationSymbol
    ) {
      references.push(node);
    }

    ts.forEachChild(node, visit);
  };

  ts.forEachChild(sourceFile, visit);
  return references;
}

function findInlineComponentClassViolations(filePath, relativeFilePath, sourceText) {
  const program = ts.createProgram([filePath], {
    allowJs: false,
    jsx: ts.JsxEmit.Preserve,
    module: ts.ModuleKind.ESNext,
    noEmit: true,
    skipLibCheck: true,
    target: ts.ScriptTarget.Latest,
  });
  const sourceFile = program.getSourceFile(filePath);

  if (!sourceFile) {
    return [];
  }

  const checker = program.getTypeChecker();
  const declarations = collectTopLevelDeclarations(sourceFile);
  const importedBindings = collectImportedBindings(sourceFile);
  const violations = [];

  for (const statement of sourceFile.statements) {
    if (!ts.isVariableStatement(statement)) {
      continue;
    }

    for (const declaration of statement.declarationList.declarations) {
      if (!isClassConstantCandidate(statement, declaration)) {
        continue;
      }

      if (!isStaticClassExpression(declaration.initializer, declarations, importedBindings)) {
        continue;
      }

      const constantName = declaration.name.text;
      const references = collectReferences(
        sourceFile,
        constantName,
        declaration.name,
        checker,
      );
      if (references.length !== 1 || !isDirectClassNameReference(references[0])) {
        continue;
      }

      violations.push({
        constantName,
        position: indexToPosition(sourceText, declaration.name.getStart(sourceFile)),
      });
    }
  }

  return violations;
}

export async function scanInlineComponentClassUsage(
  rootDirectory = sourceDir,
  allowlist = allowlistedInlineComponentClassUsage,
) {
  const sourceFiles = await collectSourceFiles(rootDirectory);
  const rootUiDirectory = path.dirname(rootDirectory);
  const allowlistedKeys = new Set(allowlist.map((entry) => toPosixPath(entry)));
  const observedAllowlistedKeys = new Set();
  const violations = [];

  for (const sourceFile of sourceFiles.sort()) {
    const relativeFilePath = toUiRelativePath(sourceFile, rootUiDirectory);
    const sourceText = await readFile(sourceFile, "utf8");

    for (const violation of findInlineComponentClassViolations(
      sourceFile,
      relativeFilePath,
      sourceText,
    )) {
      const allowlistKey = createAllowlistKey(relativeFilePath, violation.constantName);
      if (allowlistedKeys.has(allowlistKey)) {
        observedAllowlistedKeys.add(allowlistKey);
        continue;
      }

      violations.push({
        ...violation,
        allowlistKey,
        filePath: sourceFile,
        relativeFilePath,
      });
    }
  }

  const staleAllowlistEntries = [...allowlistedKeys]
    .filter((entry) => !observedAllowlistedKeys.has(entry))
    .sort((left, right) => left.localeCompare(right));

  return {
    staleAllowlistEntries,
    violations,
  };
}

function formatViolation(violation) {
  return [
    `${violation.relativeFilePath}:${violation.position.line}:${violation.position.column}`,
    `  ${violation.constantName}`,
    "  Inline this single-use static class constant directly into the nearby JSX className or remove the allowlist entry after cleanup.",
  ].join("\n");
}

function formatStaleAllowlistEntry(entry) {
  return [
    entry,
    "  Allowlist entry is stale. Remove it in the same change that inlined or deleted the class constant.",
  ].join("\n");
}

async function main() {
  const report = await scanInlineComponentClassUsage(sourceDir, getConfiguredAllowlist());
  if (report.violations.length === 0 && report.staleAllowlistEntries.length === 0) {
    return;
  }

  const violationReport = report.violations.map((violation) => formatViolation(violation)).join("\n\n");
  const staleAllowlistReport = report.staleAllowlistEntries
    .map((entry) => formatStaleAllowlistEntry(entry))
    .join("\n\n");

  console.error(
    [
      "Inline component class usage guard failed.",
      "Unexported static class constants that are referenced exactly once as a direct JSX className should be inlined for local readability.",
      violationReport.length > 0 ? ["Violations:", violationReport].join("\n\n") : "",
      staleAllowlistReport.length > 0 ? ["Stale allowlist entries:", staleAllowlistReport].join("\n\n") : "",
    ]
      .filter(Boolean)
      .join("\n\n"),
  );
  process.exitCode = 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
