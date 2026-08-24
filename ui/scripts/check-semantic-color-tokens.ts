import { readdir, readFile, stat } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { forbiddenFoundationTokenNames } from "./semantic-color-foundation-policy";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const sourceDir = process.env.AGENT_FACTORY_UI_SRC_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_SRC_DIR)
  : path.join(defaultUiDir, "src");
const sourceExtensions = new Set([".ts", ".tsx"]);
const skippedFileSuffixes = [".test.ts", ".test.tsx", ".stories.tsx"];
const skippedDirectoryNames = new Set(["generated"]);
const skippedPathFragments = [
  `${path.sep}api${path.sep}generated${path.sep}`,
  `${path.sep}components${path.sep}dashboard${path.sep}fixtures${path.sep}`,
  `${path.sep}testing${path.sep}`,
  `${path.sep}stories${path.sep}`,
  `${path.sep}messages${path.sep}`,
];
const semanticColorExceptionMarker =
  "semantic-color-exception: system-integration";
const forbiddenFoundationTokenPattern = forbiddenFoundationTokenNames.join("|");
const colorUtilityWithAlphaPattern =
  /(?:^|[\s"'`])((?:[a-z-]+:)*(?:bg|text|border|fill|stroke|decoration|outline|ring|ring-offset)-[^\s"'`]+\/\d{1,3}\b)/g;
const opacityUtilityPattern =
  /(?:^|[\s"'`])((?:[a-z-]+:)*opacity-(\d{1,3})\b)/g;
const brightnessUtilityPattern =
  /(?:^|[\s"'`])((?:[a-z-]+:)*brightness-(\d{1,3})\b)/g;
const rgbFromVarAlphaPattern = /rgb\(from[^\n]*?\/[^\n]*?\)/g;
const foundationTokenBoundary = "(?![a-z0-9-])";
const forbiddenFoundationUtilityPattern = new RegExp(
  `(?:^|[\\s"'\\\`])((?:[a-z-]+:)*(?:bg|text|border|fill|stroke|decoration|outline|ring|ring-offset)-(?:${forbiddenFoundationTokenPattern})${foundationTokenBoundary})`,
  "g",
);
const forbiddenFoundationVarPattern = new RegExp(
  String.raw`var\(--color-(?:${forbiddenFoundationTokenPattern})${foundationTokenBoundary}[^)]*\)`,
  "g",
);

export interface SemanticColorTokenViolation {
  filePath: string;
  kind:
    | "alpha-color-utility"
    | "alpha-color-expression"
    | "filter-color-utility"
    | "foundation-color-token"
    | "opacity-utility";
  message: string;
  position: {
    column: number;
    line: number;
  };
  token: string;
}

export interface SemanticColorSourceRoot {
  reportDirectory: string;
  sourceDirectory: string;
}

export type RootedSemanticColorTokenViolation = SemanticColorTokenViolation & {
  rootDirectory: string;
};

async function isDirectory(directory: string) {
  try {
    return (await stat(directory)).isDirectory();
  } catch {
    return false;
  }
}

export async function discoverSemanticColorSourceRoots(
  focusedSourceDirectory: string | null | undefined = process.env
    .AGENT_FACTORY_UI_SRC_DIR,
  uiDirectory = defaultUiDir,
): Promise<SemanticColorSourceRoot[]> {
  const resolvedUiDirectory = path.resolve(uiDirectory);
  if (focusedSourceDirectory) {
    const sourceDirectory = path.resolve(focusedSourceDirectory);
    return [
      {
        reportDirectory: path.dirname(sourceDirectory),
        sourceDirectory,
      },
    ];
  }

  const sourceRoots: SemanticColorSourceRoot[] = [
    {
      reportDirectory: resolvedUiDirectory,
      sourceDirectory: path.join(resolvedUiDirectory, "src"),
    },
  ];
  const packagesDirectory = path.join(resolvedUiDirectory, "packages");

  if (!(await isDirectory(packagesDirectory))) {
    return sourceRoots;
  }

  const packageEntries = await readdir(packagesDirectory, {
    withFileTypes: true,
  });
  for (const packageEntry of packageEntries
    .filter((entry) => entry.isDirectory())
    .sort((left, right) => left.name.localeCompare(right.name))) {
    const sourceDirectory = path.join(
      packagesDirectory,
      packageEntry.name,
      "src",
    );
    if (!(await isDirectory(sourceDirectory))) {
      continue;
    }

    sourceRoots.push({
      reportDirectory: resolvedUiDirectory,
      sourceDirectory,
    });
  }

  return sourceRoots;
}

function shouldSkipFile(filePath: string) {
  if (!sourceExtensions.has(path.extname(filePath))) {
    return path.basename(filePath) !== "styles.css";
  }

  if (skippedFileSuffixes.some((suffix) => filePath.endsWith(suffix))) {
    return true;
  }

  return skippedPathFragments.some((fragment) => filePath.includes(fragment));
}

async function collectSourceFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const files: string[] = [];

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

  return files.sort();
}

function indexToPosition(sourceText: string, index: number) {
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

function hasExceptionMarker(sourceLines: string[], lineNumber: number) {
  const candidateLines = sourceLines.slice(
    Math.max(0, lineNumber - 2),
    lineNumber,
  );
  return candidateLines.some((line) =>
    line.includes(semanticColorExceptionMarker),
  );
}

function createViolation(
  filePath: string,
  kind: SemanticColorTokenViolation["kind"],
  message: string,
  token: string,
  tokenIndex: number,
  sourceText: string,
): SemanticColorTokenViolation {
  return {
    filePath,
    kind,
    message,
    position: indexToPosition(sourceText, tokenIndex),
    token,
  };
}

function collectPatternViolations(
  sourceText: string,
  sourceLines: string[],
  pattern: RegExp,
  createMatchViolation: (
    match: RegExpMatchArray,
    token: string,
    tokenIndex: number,
  ) => SemanticColorTokenViolation | null,
) {
  const violations: SemanticColorTokenViolation[] = [];

  for (const match of sourceText.matchAll(pattern)) {
    const token = match[1] ?? match[0];
    const tokenIndex = (match.index ?? 0) + match[0].indexOf(token);
    const position = indexToPosition(sourceText, tokenIndex);
    if (hasExceptionMarker(sourceLines, position.line)) {
      continue;
    }

    const violation = createMatchViolation(match, token, tokenIndex);
    if (violation) {
      violations.push(violation);
    }
  }

  return violations;
}

function findViolationsInSource(
  sourceText: string,
  filePath: string,
): SemanticColorTokenViolation[] {
  const sourceLines = sourceText.split("\n");
  const violations: SemanticColorTokenViolation[] = [];

  violations.push(
    ...collectPatternViolations(
      sourceText,
      sourceLines,
      colorUtilityWithAlphaPattern,
      (_match, token, tokenIndex) =>
        createViolation(
          filePath,
          "alpha-color-utility",
          "Slash-opacity color utilities are not allowed in component-facing ui/src code. Use semantic tokens such as af-text-muted, af-surface-subtle, af-border, or a named token in ui/src/styles.css instead.",
          token,
          tokenIndex,
          sourceText,
        ),
    ),
    ...collectPatternViolations(
      sourceText,
      sourceLines,
      opacityUtilityPattern,
      (match, token, tokenIndex) => {
        const opacityValue = Number(match[2]);
        if (opacityValue === 0 || opacityValue === 100) {
          return null;
        }

        return createViolation(
          filePath,
          "opacity-utility",
          "Opacity utilities that encode component color emphasis are not allowed here. Use semantic text, surface, border, or disabled-state tokens instead.",
          token,
          tokenIndex,
          sourceText,
        );
      },
    ),
    ...collectPatternViolations(
      sourceText,
      sourceLines,
      brightnessUtilityPattern,
      (_match, token, tokenIndex) =>
        createViolation(
          filePath,
          "filter-color-utility",
          "Filter-based color math such as brightness-* is not allowed in component-facing ui/src code. Define a semantic hover or emphasis token in ui/src/styles.css and consume it through approved utilities instead.",
          token,
          tokenIndex,
          sourceText,
        ),
    ),
    ...collectPatternViolations(
      sourceText,
      sourceLines,
      forbiddenFoundationUtilityPattern,
      (_match, token, tokenIndex) =>
        createViolation(
          filePath,
          "foundation-color-token",
          "Foundation or alias color tokens are not allowed in component-facing ui/src code. Replace them with approved semantic roles such as af-text, af-surface, af-danger-text, af-success-text, af-warning-text, af-info-text, or another centrally defined semantic token.",
          token,
          tokenIndex,
          sourceText,
        ),
    ),
  );

  if (path.basename(filePath) !== "styles.css") {
    violations.push(
      ...collectNonStyleViolations(sourceText, filePath, sourceLines),
    );
  }

  return violations;
}

function collectNonStyleViolations(
  sourceText: string,
  filePath: string,
  sourceLines: string[],
) {
  return [
    ...collectPatternViolations(
      sourceText,
      sourceLines,
      rgbFromVarAlphaPattern,
      (_match, token, tokenIndex) => {
        if (!token.includes("--color-")) {
          return null;
        }

        return createViolation(
          filePath,
          "alpha-color-expression",
          "Component-local color alpha math is not allowed in ui/src code. Move the semantic or system-integration token definition into ui/src/styles.css and consume it through var(--color-...) instead.",
          token,
          tokenIndex,
          sourceText,
        );
      },
    ),
    ...collectPatternViolations(
      sourceText,
      sourceLines,
      forbiddenFoundationVarPattern,
      (_match, token, tokenIndex) =>
        createViolation(
          filePath,
          "foundation-color-token",
          "Foundation or alias CSS color variables are not allowed in component-facing ui/src code. Move the semantic mapping into ui/src/styles.css and consume the approved semantic token instead.",
          token,
          tokenIndex,
          sourceText,
        ),
    ),
  ];
}

export async function scanSemanticColorTokens(rootDirectory = sourceDir) {
  const sourceFiles = await collectSourceFiles(rootDirectory);
  const violations: SemanticColorTokenViolation[] = [];

  for (const sourceFile of sourceFiles) {
    const sourceText = await readFile(sourceFile, "utf8");
    violations.push(...findViolationsInSource(sourceText, sourceFile));
  }

  return violations.sort((left, right) => {
    if (left.filePath !== right.filePath) {
      return left.filePath.localeCompare(right.filePath);
    }
    if (left.position.line !== right.position.line) {
      return left.position.line - right.position.line;
    }
    return left.position.column - right.position.column;
  });
}

export async function scanSemanticColorTokensInRoots(
  sourceRoots: readonly SemanticColorSourceRoot[],
): Promise<RootedSemanticColorTokenViolation[]> {
  const violations: RootedSemanticColorTokenViolation[] = [];

  for (const sourceRoot of sourceRoots) {
    const rootViolations = await scanSemanticColorTokens(
      sourceRoot.sourceDirectory,
    );
    violations.push(
      ...rootViolations.map((violation) => ({
        ...violation,
        rootDirectory: sourceRoot.sourceDirectory,
      })),
    );
  }

  return violations.sort((left, right) => {
    if (left.filePath !== right.filePath) {
      return left.filePath.localeCompare(right.filePath);
    }
    if (left.position.line !== right.position.line) {
      return left.position.line - right.position.line;
    }
    return left.position.column - right.position.column;
  });
}

function formatViolation(
  reportDirectory: string,
  violation: SemanticColorTokenViolation,
) {
  const relativeFilePath = path
    .relative(reportDirectory, violation.filePath)
    .split(path.sep)
    .join("/");
  return [
    `${relativeFilePath}:${violation.position.line}:${violation.position.column}`,
    `  ${violation.token}`,
    `  ${violation.message}`,
  ].join("\n");
}

async function main() {
  const sourceRoots = await discoverSemanticColorSourceRoots();
  const violations = await scanSemanticColorTokensInRoots(sourceRoots);

  if (violations.length === 0) {
    const rootLabels = sourceRoots
      .map((sourceRoot) =>
        path
          .relative(
            sourceRoots[0]?.reportDirectory ?? defaultUiDir,
            sourceRoot.sourceDirectory,
          )
          .split(path.sep)
          .join("/"),
      )
      .join(", ");
    console.log(
      `Semantic color token guard passed: ${rootLabels} (0 violations).`,
    );
    return;
  }

  const report = violations
    .map((violation) => {
      const sourceRoot = sourceRoots.find(
        (candidate) => candidate.sourceDirectory === violation.rootDirectory,
      );
      return formatViolation(
        sourceRoot?.reportDirectory ?? path.dirname(violation.rootDirectory),
        violation,
      );
    })
    .join("\n\n");
  console.error(
    [
      "Semantic color token guard failed.",
      `Checked source roots: ${sourceRoots
        .map((sourceRoot) =>
          path
            .relative(
              sourceRoots[0]?.reportDirectory ?? defaultUiDir,
              sourceRoot.sourceDirectory,
            )
            .split(path.sep)
            .join("/"),
        )
        .join(", ")}`,
      "Use semantic color tokens in component-facing ui/src code and keep local alpha math inside the shared token owner when a new integration token is needed.",
      `Document rare integration-only exceptions with \`${semanticColorExceptionMarker}\` immediately above the usage.`,
      report,
    ].join("\n\n"),
  );
  process.exitCode = 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
