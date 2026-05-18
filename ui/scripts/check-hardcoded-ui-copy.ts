import { readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const UI_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const SOURCE_ROOT = path.join(UI_DIR, "src");
const BASELINE_PATH = path.join(
  UI_DIR,
  "scripts",
  "hardcoded-ui-copy-baseline.txt",
);

const TEXTUAL_JSX_ATTRIBUTE_NAMES = new Set([
  "alt",
  "aria-description",
  "aria-label",
  "aria-placeholder",
  "aria-roledescription",
  "placeholder",
  "title",
]);

const EXCLUDED_RELATIVE_PATH_PATTERNS = [
  /^src\/api\/generated\//,
  /^src\/components\/dashboard\/fixtures\//,
  /^src\/testing\//,
  /\/messages\//,
  /\.stories\./,
  /\.test\./,
] as const;

export interface HardcodedCopyFinding {
  file: string;
  line: number;
  column: number;
  kind: "jsx-attribute" | "jsx-text";
  text: string;
}

export function isExcludedSourceFile(relativePath: string): boolean {
  return EXCLUDED_RELATIVE_PATH_PATTERNS.some((pattern) =>
    pattern.test(relativePath),
  );
}

export function serializeFinding(finding: HardcodedCopyFinding): string {
  return [
    finding.file,
    finding.line,
    finding.column,
    finding.kind,
    finding.text,
  ].join("|");
}

export function parseBaselineEntries(content: string): string[] {
  return content
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0 && !line.startsWith("#"));
}

export function diffBaseline(
  findings: readonly HardcodedCopyFinding[],
  baselineEntries: readonly string[],
): {
  staleEntries: string[];
  unexpectedFindings: HardcodedCopyFinding[];
} {
  const findingMap = new Map(
    findings.map((finding) => [serializeFinding(finding), finding]),
  );
  const findingEntries = new Set(findingMap.keys());
  const baselineSet = new Set(baselineEntries);

  const unexpectedFindings = [...findingEntries]
    .filter((entry) => !baselineSet.has(entry))
    .map((entry) => findingMap.get(entry))
    .filter((finding): finding is HardcodedCopyFinding => finding !== undefined)
    .sort(compareFindings);

  const staleEntries = [...baselineSet]
    .filter((entry) => !findingEntries.has(entry))
    .sort();

  return {
    staleEntries,
    unexpectedFindings,
  };
}

export function scanSourceTextForHardcodedCopy(
  relativePath: string,
  sourceText: string,
): HardcodedCopyFinding[] {
  if (isExcludedSourceFile(relativePath)) {
    return [];
  }

  const sourceFile = ts.createSourceFile(
    relativePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    relativePath.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  const findings: HardcodedCopyFinding[] = [];

  const visit = (node: ts.Node) => {
    if (ts.isJsxText(node)) {
      const normalizedText = normalizeText(node.getText(sourceFile));
      if (looksLikeUserFacingCopy(normalizedText)) {
        findings.push(
          createFinding(
            sourceFile,
            node.getStart(sourceFile),
            "jsx-text",
            normalizedText,
          ),
        );
      }
    }

    if (ts.isJsxAttribute(node)) {
      const attributeName = node.name.text;
      if (TEXTUAL_JSX_ATTRIBUTE_NAMES.has(attributeName)) {
        const attributeValue = getJsxAttributeLiteralValue(node.initializer);
        if (looksLikeUserFacingCopy(attributeValue)) {
          findings.push(
            createFinding(
              sourceFile,
              node.initializer?.getStart(sourceFile) ??
                node.getStart(sourceFile),
              "jsx-attribute",
              attributeValue,
            ),
          );
        }
      }
    }

    ts.forEachChild(node, visit);
  };

  visit(sourceFile);
  return findings.sort(compareFindings);
}

async function collectSourceFiles(rootDir: string): Promise<string[]> {
  const entries = await readdir(rootDir, { withFileTypes: true });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const absolutePath = path.join(rootDir, entry.name);
      if (entry.isDirectory()) {
        return collectSourceFiles(absolutePath);
      }
      if (!entry.isFile()) {
        return [];
      }
      if (!absolutePath.endsWith(".ts") && !absolutePath.endsWith(".tsx")) {
        return [];
      }
      return [absolutePath];
    }),
  );

  return files.flat().sort();
}

async function loadBaselineEntries(): Promise<string[]> {
  const baselineContent = await readFile(BASELINE_PATH, "utf8");
  return parseBaselineEntries(baselineContent);
}

async function scanWorkspaceForHardcodedCopy(): Promise<
  HardcodedCopyFinding[]
> {
  const sourceFiles = await collectSourceFiles(SOURCE_ROOT);
  const findings = await Promise.all(
    sourceFiles.map(async (absolutePath) => {
      const relativePath = path
        .relative(UI_DIR, absolutePath)
        .replaceAll(path.sep, "/");
      const sourceText = await readFile(absolutePath, "utf8");
      return scanSourceTextForHardcodedCopy(relativePath, sourceText);
    }),
  );

  return findings.flat().sort(compareFindings);
}

function compareFindings(
  left: HardcodedCopyFinding,
  right: HardcodedCopyFinding,
): number {
  return (
    left.file.localeCompare(right.file) ||
    left.line - right.line ||
    left.column - right.column ||
    left.kind.localeCompare(right.kind) ||
    left.text.localeCompare(right.text)
  );
}

function createFinding(
  sourceFile: ts.SourceFile,
  position: number,
  kind: HardcodedCopyFinding["kind"],
  text: string,
): HardcodedCopyFinding {
  const { line, character } =
    sourceFile.getLineAndCharacterOfPosition(position);
  return {
    file: sourceFile.fileName,
    line: line + 1,
    column: character + 1,
    kind,
    text,
  };
}

function getJsxAttributeLiteralValue(
  initializer: ts.JsxAttributeValue | undefined,
): string {
  if (!initializer) {
    return "";
  }

  if (ts.isStringLiteral(initializer)) {
    return normalizeText(initializer.text);
  }

  if (
    ts.isJsxExpression(initializer) &&
    initializer.expression &&
    (ts.isStringLiteral(initializer.expression) ||
      ts.isNoSubstitutionTemplateLiteral(initializer.expression))
  ) {
    return normalizeText(initializer.expression.text);
  }

  return "";
}

function looksLikeUserFacingCopy(value: string): boolean {
  if (value.length < 2) {
    return false;
  }
  if (!/\p{L}/u.test(value)) {
    return false;
  }
  if (/^[A-Z]$/.test(value)) {
    return false;
  }
  return true;
}

function normalizeText(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

async function main(): Promise<void> {
  const shouldWriteBaseline = process.argv.includes("--write-baseline");
  const findings = await scanWorkspaceForHardcodedCopy();

  if (shouldWriteBaseline) {
    const nextContent = [
      "# Baseline for the hardcoded UI copy check.",
      "# Entries are path|line|column|kind|text.",
      ...findings.map(serializeFinding),
      "",
    ].join("\n");
    await writeFile(BASELINE_PATH, nextContent, "utf8");
    return;
  }

  const baselineEntries = await loadBaselineEntries();
  const { staleEntries, unexpectedFindings } = diffBaseline(
    findings,
    baselineEntries,
  );

  if (unexpectedFindings.length === 0 && staleEntries.length === 0) {
    return;
  }

  if (unexpectedFindings.length > 0) {
    console.error(
      "New hardcoded UI copy was found in production ui/src files. Move user-facing copy into a feature-owned catalog or update the documented baseline only for an intentional exception.",
    );
    for (const finding of unexpectedFindings) {
      console.error(
        `- ${finding.file}:${finding.line}:${finding.column} [${finding.kind}] ${finding.text}`,
      );
    }
  }

  if (staleEntries.length > 0) {
    console.error(
      "The hardcoded UI copy baseline has stale entries. Remove them or refresh the baseline after intentional cleanup.",
    );
    for (const entry of staleEntries) {
      console.error(`- ${entry}`);
    }
  }

  process.exit(1);
}

if (import.meta.main) {
  await main();
}
