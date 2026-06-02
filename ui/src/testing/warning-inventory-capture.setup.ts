import { appendFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { afterEach, beforeEach } from "vitest";

const outDir = join(process.cwd(), ".warning-inventory");
const outFile = join(outDir, "console-entries.jsonl");

mkdirSync(outDir, { recursive: true });
if (process.env.VITEST_WARNING_INVENTORY_APPEND !== "1") {
  writeFileSync(outFile, "");
}

type ConsoleLevel = "warn" | "error";

type ConsoleEntry = {
  level: ConsoleLevel;
  message: string;
  testFile?: string;
  testName?: string;
};

let currentTestFile: string | undefined;
let currentTestName: string | undefined;

function formatArgs(args: unknown[]): string {
  return args
    .map((value) => {
      if (typeof value === "string") {
        return value;
      }
      if (value instanceof Error) {
        return value.message;
      }
      try {
        return JSON.stringify(value);
      } catch {
        return String(value);
      }
    })
    .join(" ");
}

function record(level: ConsoleLevel, args: unknown[]): void {
  const entry: ConsoleEntry = {
    level,
    message: formatArgs(args),
    testFile: currentTestFile,
    testName: currentTestName,
  };
  appendFileSync(outFile, `${JSON.stringify(entry)}\n`);
}

beforeEach((context) => {
  currentTestFile = context.task.file?.filepath;
  currentTestName = context.task.name;
});

afterEach(() => {
  currentTestFile = undefined;
  currentTestName = undefined;
});

for (const level of ["warn", "error"] as const) {
  const original = console[level].bind(console);
  console[level] = (...args: unknown[]) => {
    record(level, args);
    original(...args);
  };
}
