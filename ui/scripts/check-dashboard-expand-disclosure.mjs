import { readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { dashboardExpandDisclosureGuardPaths } from "./dashboard-expand-disclosure-guard-paths.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");

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

function hasJsxAttribute(node, attributeName) {
  return node.attributes.properties.some((property) => {
    if (!ts.isJsxAttribute(property)) {
      return false;
    }

    if (ts.isIdentifier(property.name)) {
      return property.name.text === attributeName;
    }

    return (
      ts.isJsxNamespacedName(property.name) &&
      property.name.name.text === attributeName
    );
  });
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

function collectDisclosureUsage(sourceText, filePath) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    getScriptKind(filePath),
  );
  const imports = {
    disclosureButton: false,
    currentSelectionExpandableSection: false,
    expandablePanelIcon: false,
    expandablePanelTrigger: false,
    standardExpandableSection: false,
  };
  const rawDisclosureButtons = [];

  function visit(node) {
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      const modulePath = node.moduleSpecifier.text;

      if (modulePath.includes("disclosure-button")) {
        imports.disclosureButton = true;
      }

      if (modulePath.includes("expandable-panel-icon")) {
        imports.expandablePanelIcon = true;
      }

      if (modulePath.includes("expandable-panel-trigger")) {
        imports.expandablePanelTrigger = true;
      }

      if (modulePath.includes("standard-card-components")) {
        imports.standardExpandableSection = true;
      }

      if (modulePath.includes("detail-card-shared")) {
        const importClause = node.importClause;
        if (
          importClause?.namedBindings &&
          ts.isNamedImports(importClause.namedBindings)
        ) {
          for (const element of importClause.namedBindings.elements) {
            if (element.name.text === "CurrentSelectionExpandableSection") {
              imports.currentSelectionExpandableSection = true;
            }
          }
        }
      }

      if (
        modulePath.endsWith("/components/ui") ||
        modulePath.endsWith("/components/ui/index")
      ) {
        const importClause = node.importClause;
        if (
          importClause?.namedBindings &&
          ts.isNamedImports(importClause.namedBindings)
        ) {
          for (const element of importClause.namedBindings.elements) {
            const importedName = element.name.text;
            if (importedName === "ExpandablePanelTrigger") {
              imports.expandablePanelTrigger = true;
            }
            if (importedName === "ExpandablePanelIcon") {
              imports.expandablePanelIcon = true;
            }
            if (importedName === "DisclosureButton") {
              imports.disclosureButton = true;
            }
            if (importedName === "StandardExpandableSection") {
              imports.standardExpandableSection = true;
            }
          }
        }
      }
    }

    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      getJsxTagName(node.tagName) === "button" &&
      hasJsxAttribute(node, "aria-expanded")
    ) {
      rawDisclosureButtons.push(
        indexToPosition(sourceFile, node.getStart(sourceFile)),
      );
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);

  return {
    imports,
    rawDisclosureButtons,
  };
}

function getOwnerViolation({ imports, owner, relativeFilePath }) {
  if (owner === "expandable-panel-trigger") {
    if (
      !imports.expandablePanelTrigger &&
      !imports.standardExpandableSection &&
      !imports.currentSelectionExpandableSection
    ) {
      return {
        reason:
          "Migrated dashboard inline expand entry points must import and use ExpandablePanelTrigger, StandardExpandableSection, or CurrentSelectionExpandableSection.",
        recommendedFix:
          "Replace ad-hoc disclosure buttons with ExpandablePanelTrigger, StandardExpandableSection, or CurrentSelectionExpandableSection and keep expanded state local to the feature.",
        relativeFilePath,
      };
    }

    if (imports.disclosureButton) {
      return {
        reason:
          "Direct DisclosureButton ownership is reserved for ExpandablePanelTrigger and the workflow-activity legend shell exception.",
        recommendedFix:
          "Route disclosure through ExpandablePanelTrigger instead of importing DisclosureButton in this migrated path.",
        relativeFilePath,
      };
    }
  }

  if (owner === "expandable-panel-icon-shell") {
    if (!imports.expandablePanelIcon) {
      return {
        reason:
          "Workflow-activity legend expand chrome must keep ExpandablePanelIcon for consistent chevron treatment.",
        recommendedFix:
          "Import ExpandablePanelIcon from the shared UI surface and pair it with DisclosureButton for legend pill styling.",
        relativeFilePath,
      };
    }

    if (!imports.disclosureButton) {
      return {
        reason:
          "Workflow-activity legend expand chrome must keep DisclosureButton as the semantic shell around ExpandablePanelIcon.",
        recommendedFix:
          "Use DisclosureButton + ExpandablePanelIcon instead of raw buttons or ExpandablePanelTrigger variants in legend chrome.",
        relativeFilePath,
      };
    }

    if (imports.expandablePanelTrigger) {
      return {
        reason:
          "Legend pill styling conflicts with ExpandablePanelTrigger variant classes; keep the icon-shell pattern here.",
        recommendedFix:
          "Remove ExpandablePanelTrigger from dashboard-flow-axis-legend and keep DisclosureButton + ExpandablePanelIcon.",
        relativeFilePath,
      };
    }
  }

  return null;
}

export async function scanDashboardExpandDisclosure(
  uiDirectory = defaultUiDir,
  guardPaths = dashboardExpandDisclosureGuardPaths,
) {
  const violations = [];

  for (const entry of guardPaths) {
    const filePath = path.join(uiDirectory, entry.relativeFilePath);
    const sourceText = await readFile(filePath, "utf8").catch(() => null);

    if (sourceText === null) {
      violations.push({
        details: "Guarded file is missing from the worktree.",
        kind: "missing-file",
        relativeFilePath: entry.relativeFilePath,
      });
      continue;
    }

    const usage = collectDisclosureUsage(sourceText, filePath);
    const ownerViolation = getOwnerViolation({
      imports: usage.imports,
      owner: entry.owner,
      relativeFilePath: entry.relativeFilePath,
    });

    if (ownerViolation) {
      violations.push({
        ...ownerViolation,
        kind: "owner",
      });
    }

    if (usage.rawDisclosureButtons.length > 0) {
      violations.push({
        details: `raw aria-expanded buttons at ${usage.rawDisclosureButtons
          .map((position) => `${position.line}:${position.column}`)
          .join(", ")}`,
        kind: "raw-disclosure-button",
        reason:
          "Migrated dashboard inline expand paths must not add raw <button aria-expanded> disclosure controls.",
        recommendedFix:
          "Use ExpandablePanelTrigger (or DisclosureButton + ExpandablePanelIcon for legend chrome) instead of raw disclosure buttons.",
        relativeFilePath: entry.relativeFilePath,
      });
    }
  }

  return { violations };
}

function formatViolation(violation) {
  return [
    violation.relativeFilePath,
    `  kind: ${violation.kind}`,
    violation.details ? `  details: ${violation.details}` : "",
    violation.reason ? `  reason: ${violation.reason}` : "",
    violation.recommendedFix ? `  fix: ${violation.recommendedFix}` : "",
  ]
    .filter(Boolean)
    .join("\n");
}

async function main() {
  const report = await scanDashboardExpandDisclosure();

  if (report.violations.length === 0) {
    return;
  }

  console.error(
    [
      "Dashboard expand disclosure guard failed.",
      "Migrated dashboard inline expand entry points must stay on ExpandablePanelTrigger, StandardExpandableSection, CurrentSelectionExpandableSection, or the ExpandablePanelIcon legend shell exception.",
      "Violations:",
      report.violations.map(formatViolation).join("\n\n"),
    ].join("\n\n"),
  );
  process.exitCode = 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
