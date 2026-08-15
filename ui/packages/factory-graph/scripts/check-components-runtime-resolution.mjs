import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

// Keep this probe relative to the factory-graph package so Bun loads this
// package's tsconfig paths. An absolute UI test path would select the UI
// tsconfig instead and would not protect the package-local alias contract.
const preloadPath = "../../src/testing/bun/component.setup.ts";
const testPath =
  "../../src/features/factory-graph-editor/components/controls/factory-graph-editor-controls.bun.component.test.tsx";
const bunExecutable = process.env.BUN_EXECUTABLE ?? "bun";
const bunArguments = [
  "test",
  "--preload",
  preloadPath,
  "--reporter=dots",
  "--timeout=10000",
  testPath,
];

function assertSourceEntrypoints(tsconfig) {
  const paths = tsconfig.compilerOptions?.paths ?? {};
  const expectedPaths = {
    "@you-agent-factory/components": ["../components/src/index.ts"],
    "@you-agent-factory/components/*": ["../components/src/*"],
  };

  for (const [specifier, expected] of Object.entries(expectedPaths)) {
    if (JSON.stringify(paths[specifier]) === JSON.stringify(expected)) {
      continue;
    }

    throw new Error(
      `[factory-graph-components-runtime] ${specifier} must resolve to ${expected[0]} in the package-local tsconfig; found ${JSON.stringify(paths[specifier]) ?? "missing"}.`,
    );
  }
}

function outputFrom(error) {
  return [error.stdout, error.stderr].filter(Boolean).join("\n");
}

try {
  const tsconfig = JSON.parse(
    await readFile(path.join(packageRoot, "tsconfig.json"), "utf8"),
  );
  assertSourceEntrypoints(tsconfig);

  const { stdout, stderr } = await execFileAsync(bunExecutable, bunArguments, {
    cwd: packageRoot,
    maxBuffer: 10 * 1024 * 1024,
    windowsHide: true,
  });
  const output = [stdout, stderr].filter(Boolean).join("\n");
  const passCount = Number(output.match(/(?:^|\n)\s*(\d+)\s+pass\b/)?.[1]);

  process.stdout.write(output);
  if (!Number.isInteger(passCount) || passCount < 1) {
    throw new Error(
      "[factory-graph-components-runtime] expected the focused component test to execute at least one test; received output without a positive pass count.",
    );
  }

  process.stdout.write(
    `[factory-graph-components-runtime] passed (${passCount} focused tests executed under the factory-graph package tsconfig).\n`,
  );
} catch (error) {
  const output = outputFrom(error);
  process.stderr.write(
    `${[
      "[factory-graph-components-runtime] focused component resolution check failed:",
      output,
    ]
      .filter(Boolean)
      .join("\n")}\n`,
  );
  process.exitCode = Number.isInteger(error?.code) ? error.code : 1;
}
