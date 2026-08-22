import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import test from "node:test";

const execFileAsync = promisify(execFile);
const biomeCli = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../node_modules/@biomejs/biome/bin/biome",
);
const generatedOutputDirectories = [
  ".vitest-reports",
  "coverage",
  "dist",
  "storybook-static",
];

const topLevelOnlyExclusions = [
  "!.vitest-reports",
  "!coverage",
  "!dist",
  "!storybook-static",
];
const recursiveExclusions = topLevelOnlyExclusions.map((pattern) =>
  pattern.replace("!", "!**/"),
);

function configFor(exclusions) {
  return `${JSON.stringify(
    {
      files: {
        includes: ["**", ...exclusions],
      },
      linter: {
        enabled: true,
        rules: {
          complexity: {
            noUselessEmptyExport: "error",
          },
        },
      },
    },
    null,
    2,
  )}\n`;
}

async function writeFixtureFile(root, relativePath) {
  const filePath = path.join(root, relativePath);
  await mkdir(path.dirname(filePath), { recursive: true });
  await writeFile(filePath, "export type Generated = string;\nexport {};\n");
}

function parseDiagnostics(stdout) {
  const output = stdout.trim();
  if (output === "") {
    return [];
  }

  const report = JSON.parse(output);
  return Array.isArray(report) ? report : report.diagnostics;
}

async function runBiome(root) {
  let result;
  try {
    result = await execFileAsync(
      process.execPath,
      [biomeCli, "lint", "--reporter=json"],
      { cwd: root, windowsHide: true },
    );
  } catch (error) {
    result = error;
  }

  return {
    diagnostics: parseDiagnostics(result.stdout ?? ""),
    exitCode: Number(result.code ?? result.status ?? 0),
    output: `${result.stdout ?? ""}${result.stderr ?? ""}`,
  };
}

function diagnosticPath(diagnostic) {
  const locationPath = diagnostic.location?.path;
  const filePath =
    typeof locationPath === "string" ? locationPath : locationPath?.file;
  return (filePath ?? "").replaceAll("\\", "/");
}

function diagnosticsInDirectory(diagnostics, directory) {
  return diagnostics.filter((diagnostic) =>
    diagnosticPath(diagnostic).split("/").includes(directory),
  );
}

test("Biome excludes nested output directories without blinding handwritten source", async () => {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "biome-output-exclusions-"),
  );
  const configPath = path.join(tempRoot, "biome.json");
  const nestedDistPath = "packages/factory-graph/dist/generated.d.ts";

  try {
    await writeFile(configPath, configFor(topLevelOnlyExclusions));
    await writeFixtureFile(tempRoot, nestedDistPath);

    const beforeFix = await runBiome(tempRoot);
    assert.equal(beforeFix.exitCode, 1, beforeFix.output);
    assert.equal(
      diagnosticsInDirectory(beforeFix.diagnostics, "dist").length,
      1,
      beforeFix.output,
    );
    assert.match(beforeFix.diagnostics[0].category, /noUselessEmptyExport$/);

    await rm(path.join(tempRoot, "packages"), {
      force: true,
      recursive: true,
    });
    const withoutNestedDist = await runBiome(tempRoot);
    assert.equal(withoutNestedDist.exitCode, 0, withoutNestedDist.output);
    assert.equal(withoutNestedDist.diagnostics.length, 0);

    await writeFile(configPath, configFor(recursiveExclusions));
    for (const directory of generatedOutputDirectories) {
      await writeFixtureFile(
        tempRoot,
        `packages/factory-graph/${directory}/generated.d.ts`,
      );
    }

    const afterFix = await runBiome(tempRoot);
    assert.equal(afterFix.exitCode, 0, afterFix.output);
    assert.equal(afterFix.diagnostics.length, 0, afterFix.output);

    await writeFixtureFile(tempRoot, "src/handwritten.d.ts");
    const handwrittenSource = await runBiome(tempRoot);
    assert.equal(handwrittenSource.exitCode, 1, handwrittenSource.output);
    assert.equal(
      handwrittenSource.diagnostics.length,
      1,
      handwrittenSource.output,
    );
    assert.equal(
      diagnosticPath(handwrittenSource.diagnostics[0]),
      "src/handwritten.d.ts",
    );
    assert.match(
      handwrittenSource.diagnostics[0].category,
      /noUselessEmptyExport$/,
    );
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
