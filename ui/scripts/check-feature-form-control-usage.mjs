import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { approvedFeatureFormControlUsageAllowlist } from "./feature-form-control-usage-allowlist.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const sourceDir = process.env.AGENT_FACTORY_UI_SRC_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_SRC_DIR)
  : path.join(defaultUiDir, "src");
const uiDir = path.dirname(sourceDir);
const sourceExtensions = new Set([".jsx", ".tsx"]);
const skippedFileSuffixes = [".stories.tsx", ".test.tsx"];
const skippedDirectoryNames = new Set(["generated"]);
const skippedPathFragments = [`${path.sep}api${path.sep}generated${path.sep}`];
const featureComponentsPathFragment = `${path.sep}features${path.sep}`;
const featureComponentsDirectoryFragment = `${path.sep}components${path.sep}`;
const allowlistOverride =
  process.env.AGENT_FACTORY_UI_FORM_CONTROL_USAGE_ALLOWLIST;

const blockedSelectPrimitiveImports = new Set([
  "NativeSelect",
  "Select",
  "SelectContent",
  "SelectGroup",
  "SelectItem",
  "SelectLabel",
  "SelectSeparator",
  "SelectTrigger",
  "SelectValue",
]);

const blockedSelectPrimitiveJsxTags = new Set([
  "NativeSelect",
  "Select",
  "SelectContent",
  "SelectGroup",
  "SelectItem",
  "SelectLabel",
  "SelectSeparator",
  "SelectTrigger",
  "SelectValue",
  "option",
  "select",
]);

function getConfiguredAllowlist() {
  if (!allowlistOverride) {
    return approvedFeatureFormControlUsageAllowlist;
  }

  return JSON.parse(allowlistOverride);
}

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
}

function toUiRelativePath(filePath, rootDirectory = uiDir) {
  return toPosixPath(path.relative(rootDirectory, filePath));
}

function shouldScanFile(filePath) {
  if (!sourceExtensions.has(path.extname(filePath))) {
    return false;
  }

  if (skippedFileSuffixes.some((suffix) => filePath.endsWith(suffix))) {
    return false;
  }

  if (!filePath.includes(featureComponentsPathFragment)) {
    return false;
  }

  if (!filePath.includes(featureComponentsDirectoryFragment)) {
    return false;
  }

  return !skippedPathFragments.some((fragment) => filePath.includes(fragment));
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

    if (shouldScanFile(entryPath)) {
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

  return ts.ScriptKind.JSX;
}

function indexToPosition(sourceFile, start) {
  const { character, line } = sourceFile.getLineAndCharacterOfPosition(start);
  return { column: character + 1, line: line + 1 };
}

function getJsxTagName(tagName) {
  if (ts.isIdentifier(tagName)) {
    return tagName.text;
  }

  if (ts.isPropertyAccessExpression(tagName) && ts.isIdentifier(tagName.name)) {
    return tagName.name.text;
  }

  return null;
}

const componentsPackageModule = "@you-agent-factory/components";
const componentsPackageFormsModule = "@you-agent-factory/components/forms";

const approvedSelectHelperImports = new Set([
  "EnumSelect",
  "OptionalEnumSelect",
  "ResetEnumSelect",
  "SelectField",
]);

function isApprovedSelectHelperModulePath(modulePath) {
  return (
    modulePath === componentsPackageModule ||
    modulePath === componentsPackageFormsModule ||
    modulePath.endsWith("/enum-select") ||
    modulePath.endsWith("/components/ui") ||
    modulePath.endsWith("/components/ui/index")
  );
}

function isSelectModulePath(modulePath) {
  return (
    modulePath.includes("/native-select") ||
    modulePath.endsWith("/select") ||
    modulePath.endsWith("/select.tsx") ||
    modulePath.endsWith("/enum-select") ||
    modulePath.endsWith("/components/ui") ||
    modulePath.endsWith("/components/ui/index")
  );
}

function isRadixSelectModulePath(modulePath) {
  return modulePath === "@radix-ui/react-select";
}

function collectFormControlUsage(sourceText, filePath) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const violations = [];

  function pushViolation(kind, position, reason, recommendedFix) {
    violations.push({
      kind,
      position: indexToPosition(sourceFile, position),
      reason,
      recommendedFix,
    });
  }

  function visit(node) {
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      const modulePath = node.moduleSpecifier.text;
      const importClause = node.importClause;

      if (
        isRadixSelectModulePath(modulePath) ||
        modulePath.includes("/native-select")
      ) {
        pushViolation(
          "blocked-select-import",
          node.getStart(sourceFile),
          "Feature component code must not import Radix or native select primitives directly.",
          "Use EnumSelect, OptionalEnumSelect, ResetEnumSelect, or SelectField from @you-agent-factory/components instead.",
        );
      }

      if (
        isSelectModulePath(modulePath) ||
        isApprovedSelectHelperModulePath(modulePath)
      ) {
        const namedBindings = importClause?.namedBindings;
        if (namedBindings && ts.isNamedImports(namedBindings)) {
          for (const element of namedBindings.elements) {
            const importedName = element.name.text;
            if (
              blockedSelectPrimitiveImports.has(importedName) &&
              !approvedSelectHelperImports.has(importedName)
            ) {
              pushViolation(
                "blocked-select-import",
                element.getStart(sourceFile),
                "Feature component code must not import raw select primitives owned by components/ui.",
                "Use EnumSelect, OptionalEnumSelect, ResetEnumSelect, or SelectField from @you-agent-factory/components instead.",
              );
            }
          }
        }
      }
    }

    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      getJsxTagName(node.tagName)
    ) {
      const tagName = getJsxTagName(node.tagName);
      if (tagName && blockedSelectPrimitiveJsxTags.has(tagName)) {
        pushViolation(
          tagName === "select" || tagName === "option"
            ? "raw-select"
            : tagName === "NativeSelect"
              ? "native-select"
              : "select-primitive",
          node.getStart(sourceFile),
          tagName === "select" || tagName === "option"
            ? "Feature component code must not render raw native select markup."
            : tagName === "NativeSelect"
              ? "Feature component code must not render NativeSelect."
              : "Feature component code must not compose Radix select primitives directly.",
          "Use EnumSelect, OptionalEnumSelect, ResetEnumSelect, or SelectField from @you-agent-factory/components instead.",
        );
      }
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);

  return violations;
}

function buildAllowlistMap(
  allowlist = approvedFeatureFormControlUsageAllowlist,
) {
  return new Map(allowlist.map((entry) => [entry.relativeFilePath, entry]));
}

function getAllowedViolationKinds(allowlistEntry) {
  return new Set(allowlistEntry?.allowedKinds ?? []);
}

export async function scanFeatureFormControlUsage(
  rootDirectory = sourceDir,
  allowlist = approvedFeatureFormControlUsageAllowlist,
) {
  const sourceFiles = await collectSourceFiles(rootDirectory);
  const rootUiDirectory = path.dirname(rootDirectory);
  const allowlistMap = buildAllowlistMap(allowlist);
  const staleAllowlistEntries = [];
  const violations = [];

  for (const sourceFile of sourceFiles.sort()) {
    const sourceText = await readFile(sourceFile, "utf8");
    const fileViolations = collectFormControlUsage(sourceText, sourceFile);
    const relativeFilePath = toUiRelativePath(sourceFile, rootUiDirectory);
    const allowlistEntry = allowlistMap.get(relativeFilePath);
    const allowedKinds = getAllowedViolationKinds(allowlistEntry);

    for (const violation of fileViolations) {
      if (allowedKinds.has(violation.kind)) {
        continue;
      }

      violations.push({
        ...violation,
        filePath: sourceFile,
        relativeFilePath,
      });
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

    const fileViolations = collectFormControlUsage(sourceText, sourceFilePath);
    const allowedKinds = getAllowedViolationKinds(entry);
    const unexpectedViolations = fileViolations.filter(
      (violation) => !allowedKinds.has(violation.kind),
    );

    if (unexpectedViolations.length > 0 && allowedKinds.size > 0) {
      staleAllowlistEntries.push({
        reason: `Allowlist no longer covers all observed violations. Found ${unexpectedViolations.map((violation) => violation.kind).join(", ")}.`,
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
    `${path.relative(rootDirectory, violation.filePath)}:${violation.position.line}:${violation.position.column}`,
    `  kind: ${violation.kind}`,
    `  reason: ${violation.reason}`,
    `  fix: ${violation.recommendedFix}`,
  ].join("\n");
}

async function main() {
  const report = await scanFeatureFormControlUsage(
    sourceDir,
    getConfiguredAllowlist(),
  );

  if (
    report.violations.length === 0 &&
    report.staleAllowlistEntries.length === 0
  ) {
    return;
  }

  const violations = report.violations
    .map((violation) => formatViolation(uiDir, violation))
    .join("\n\n");
  const staleAllowlistEntries = report.staleAllowlistEntries
    .map((entry) => [entry.relativeFilePath, `  ${entry.reason}`].join("\n"))
    .join("\n\n");

  console.error(
    [
      "Feature form-control usage guard failed.",
      "Feature component code must reuse shared select helpers (EnumSelect, OptionalEnumSelect, ResetEnumSelect, SelectField) from @you-agent-factory/components instead of raw/native/Radix select primitives.",
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
