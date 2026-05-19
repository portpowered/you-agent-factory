import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const sourceDir = process.env.AGENT_FACTORY_UI_SRC_DIR
  ? path.resolve(process.env.AGENT_FACTORY_UI_SRC_DIR)
  : path.join(defaultUiDir, "src");
const uiDir = path.dirname(sourceDir);
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx"]);
const skippedFileSuffixes = [".test.js", ".test.jsx", ".test.ts", ".test.tsx", ".stories.tsx"];
const skippedDirectoryNames = new Set(["generated"]);
const skippedPathFragments = [`${path.sep}api${path.sep}generated${path.sep}`];
const intrinsicSizingExceptionMarker = "tailwind-exception: intrinsic-sizing";
const utilityPrefixPattern = [
  "p",
  "px",
  "py",
  "pt",
  "pr",
  "pb",
  "pl",
  "m",
  "mx",
  "my",
  "mt",
  "mr",
  "mb",
  "ml",
  "gap",
  "gap-x",
  "gap-y",
  "space-x",
  "space-y",
  "inset",
  "inset-x",
  "inset-y",
  "top",
  "right",
  "bottom",
  "left",
  "scroll-m",
  "scroll-mx",
  "scroll-my",
  "scroll-mt",
  "scroll-mr",
  "scroll-mb",
  "scroll-ml",
  "scroll-p",
  "scroll-px",
  "scroll-py",
  "scroll-pt",
  "scroll-pr",
  "scroll-pb",
  "scroll-pl",
  "rounded",
  "rounded-s",
  "rounded-e",
  "rounded-t",
  "rounded-r",
  "rounded-b",
  "rounded-l",
  "rounded-ss",
  "rounded-se",
  "rounded-ee",
  "rounded-es",
  "rounded-tl",
  "rounded-tr",
  "rounded-br",
  "rounded-bl",
  "w",
  "min-w",
  "max-w",
  "h",
  "min-h",
  "max-h",
].join("|");
const arbitrarySpacingPattern = new RegExp(
  String.raw`(?:^|:)(-?(?:${utilityPrefixPattern})-\[[^\]]+\])$`,
);
const customBreakpointVariantPattern = /(?:^|:)(?:max|min)-\[[^\]]+\]:/;
const customMediaVariantPattern = /(?:^|:)\[@media[^\]]+\]:/;
const tokenPattern = /[^\s"'`]+/g;
const arbitrarySizeUtilityPattern = /(?:^|:)-?(?:w|min-w|max-w|h|min-h|max-h)-\[[^\]]+\]$/;

function stripTokenPunctuation(rawToken) {
  return rawToken
    .replace(/^[{(<>,;]+/, "")
    .replace(/[)}>.,;]+$/, "");
}

function shouldSkipFile(filePath) {
  if (!sourceExtensions.has(path.extname(filePath))) {
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

function hasIntrinsicSizingException(sourceLines, lineNumber) {
  const candidateLines = sourceLines.slice(Math.max(0, lineNumber - 3), lineNumber);
  return candidateLines.some((line) => line.includes(intrinsicSizingExceptionMarker));
}

function findTailwindTokenViolations(sourceText) {
  const sourceLines = sourceText.split("\n");
  const violations = [];
  for (const match of sourceText.matchAll(tokenPattern)) {
    const rawToken = match[0];
    const token = stripTokenPunctuation(rawToken);
    const tokenIndex = match.index ?? 0;

    if (token.length === 0) {
      continue;
    }

    if (customBreakpointVariantPattern.test(token) || customMediaVariantPattern.test(token)) {
      violations.push({
        kind: "custom-breakpoint",
        message:
          "Custom responsive breakpoint variants are not allowed for ordinary layout. Use approved variants such as sm:, md:, lg:, xl:, 2xl:, or a documented named project breakpoint.",
        token,
        tokenIndex,
      });
      continue;
    }

    const arbitrarySpacingMatch = token.match(arbitrarySpacingPattern);
    if (!arbitrarySpacingMatch) {
      continue;
    }

    if (
      arbitrarySizeUtilityPattern.test(arbitrarySpacingMatch[1]) &&
      hasIntrinsicSizingException(sourceLines, indexToPosition(sourceText, tokenIndex).line)
    ) {
      continue;
    }

    violations.push({
      kind: "arbitrary-spacing",
      message:
        "Arbitrary Tailwind spacing utilities are not allowed for ordinary layout rhythm. Use an approved spacing token or document a true intrinsic sizing case with `tailwind-exception: intrinsic-sizing`.",
      token: arbitrarySpacingMatch[1],
      tokenIndex,
    });
  }

  return violations;
}

export async function scanTailwindSpacingTokens(rootDirectory = sourceDir) {
  const sourceFiles = await collectSourceFiles(rootDirectory);
  const violations = [];

  for (const sourceFile of sourceFiles.sort()) {
    const sourceText = await readFile(sourceFile, "utf8");
    for (const violation of findTailwindTokenViolations(sourceText)) {
      violations.push({
        ...violation,
        filePath: sourceFile,
        position: indexToPosition(sourceText, violation.tokenIndex),
      });
    }
  }

  return violations;
}

function formatViolation(rootDirectory, violation) {
  const relativeFilePath = path.relative(rootDirectory, violation.filePath);
  return [
    `${relativeFilePath}:${violation.position.line}:${violation.position.column}`,
    `  ${violation.token}`,
    `  ${violation.message}`,
  ].join("\n");
}

async function main() {
  const violations = await scanTailwindSpacingTokens(sourceDir);

  if (violations.length === 0) {
    return;
  }

  const report = violations
    .map((violation) => formatViolation(uiDir, violation))
    .join("\n\n");
  console.error(
    [
      "Tailwind spacing token guard failed.",
      "Use approved spacing scale utilities and standard responsive variants for ordinary UI layout.",
      report,
    ].join("\n\n"),
  );
  process.exitCode = 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
