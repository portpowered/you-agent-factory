const PRELOAD_PATH = "./src/testing/bun/component.setup.ts";
const TEST_TIMEOUT_ARGUMENT = "--timeout=10000";
const REPORTER_ARGUMENT = "--reporter=dots";

export interface BunComponentTestRun {
  exitCode: number;
  stdout: string;
  stderr: string;
}

export interface BunFailureBlock {
  file: string;
  testName: string | null;
  output: string;
}

const fileHeaderPattern = /^(.*\.(?:cjs|js|jsx|mjs|ts|tsx)):\s*$/i;
const summaryLinePattern =
  /^(?:\d+\s+(?:pass|fail|error|expect\(\) calls)|Ran\s+)/;
const testDurationSuffixPattern = /\s+\[\d+(?:\.\d+)?(?:ms|s)\]$/;

function decodeSpawnOutput(output: unknown): string {
  if (typeof output === "string") {
    return output;
  }
  if (output instanceof Uint8Array) {
    return new TextDecoder().decode(output);
  }
  return "";
}

export function buildBunComponentTestCommand(files: string[]): string[] {
  return [
    process.execPath,
    "test",
    "--preload",
    PRELOAD_PATH,
    REPORTER_ARGUMENT,
    TEST_TIMEOUT_ARGUMENT,
    ...files,
  ];
}

export function runBunComponentTests(files: string[]): BunComponentTestRun {
  const result = Bun.spawnSync({
    cmd: buildBunComponentTestCommand(files),
    cwd: process.cwd(),
    stderr: "pipe",
    stdout: "pipe",
  });

  return {
    exitCode: result.exitCode ?? 1,
    stderr: decodeSpawnOutput(result.stderr),
    stdout: decodeSpawnOutput(result.stdout),
  };
}

function isSummaryLine(line: string): boolean {
  return summaryLinePattern.test(line.trim());
}

export function parseBunFailureBlocks(output: string): BunFailureBlock[] {
  const lines = output
    .replaceAll("\r\n", "\n")
    .replaceAll("\r", "\n")
    .split("\n");
  const headers = lines
    .map((line, index) => ({ line: line.trim(), index }))
    .filter(({ line }) => fileHeaderPattern.test(line));
  const failures: BunFailureBlock[] = [];

  for (const [headerIndex, header] of headers.entries()) {
    const nextHeaderIndex = headers[headerIndex + 1]?.index ?? lines.length;
    const summaryIndex = lines.findIndex(
      (line, index) =>
        index > header.index && index < nextHeaderIndex && isSummaryLine(line),
    );
    const endIndex = summaryIndex === -1 ? nextHeaderIndex : summaryIndex;
    const blockLines = lines.slice(header.index + 1, endIndex);
    const testName =
      blockLines
        .find((line) => line.trim().startsWith("(fail) "))
        ?.trim()
        .slice(7)
        .replace(testDurationSuffixPattern, "") ?? null;

    if (
      testName ||
      blockLines.some((line) => line.includes("Unhandled error"))
    ) {
      failures.push({
        file: header.line.replace(/:\s*$/, ""),
        output: blockLines.join("\n").trim(),
        testName,
      });
    }
  }

  return failures;
}

export function formatBunFailureReport(output: string): string {
  const failures = parseBunFailureBlocks(output);
  if (failures.length === 0) {
    return "[ui-component] Bun reported a failure without a file-scoped failure block; raw output follows.";
  }

  const lines = ["[ui-component] Actionable failure details:"];
  for (const failure of failures) {
    lines.push(`- file: ${failure.file}`);
    if (failure.testName) {
      lines.push(`  full test name: ${failure.testName}`);
      lines.push("  assertion failure or diff:");
    } else {
      lines.push("  setup/runtime error:");
    }
    lines.push(...failure.output.split("\n").map((line) => `    ${line}`));
  }

  return lines.join("\n");
}
