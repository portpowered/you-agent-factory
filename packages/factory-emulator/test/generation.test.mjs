import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import process from "node:process";
import test from "node:test";

const repositoryRoot = resolve(import.meta.dirname, "../../..");
const generatedPaths = [
  "packages/factory-emulator/generated/factory-emulator-scenario.schema.json",
  "packages/factory-emulator/src/generated/scenario.ts",
  "packages/factory-emulator/src/generated/scenario-schema.js",
  "packages/factory-emulator/src/generated/scenario-schema.d.ts",
];

function generatedArtifactBytes() {
  return generatedPaths.map((path) => readFileSync(join(repositoryRoot, path)));
}

function runGeneration(...arguments_) {
  return spawnSync(
    process.execPath,
    ["scripts/generate-emulator.mjs", ...arguments_],
    {
      cwd: repositoryRoot,
      encoding: "utf8",
    },
  );
}

test("scenario generation is byte-for-byte stable on consecutive runs", () => {
  assert.equal(runGeneration().status, 0);
  const firstRun = generatedArtifactBytes();

  assert.equal(runGeneration().status, 0);
  const secondRun = generatedArtifactBytes();

  assert.deepEqual(secondRun, firstRun);
});

test("freshness verification accepts current artifacts and rejects intentional drift", () => {
  assert.equal(runGeneration("--check").status, 0);

  for (const generatedPath of generatedPaths) {
    const absolutePath = join(repositoryRoot, generatedPath);
    const original = readFileSync(absolutePath);
    try {
      writeFileSync(
        absolutePath,
        Buffer.concat([original, Buffer.from("\n// stale\n")]),
      );

      const staleResult = runGeneration("--check");
      assert.notEqual(staleResult.status, 0, `${generatedPath} was incorrectly fresh`);
      assert.match(
        `${staleResult.stdout}\n${staleResult.stderr}`,
        /Generated scenario contract artifacts are stale.*run generate.*commit/i,
      );
    } finally {
      writeFileSync(absolutePath, original);
    }
  }

  assert.equal(runGeneration("--check").status, 0);
});
