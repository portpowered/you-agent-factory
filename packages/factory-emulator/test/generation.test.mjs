import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
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

test("scenario generation is byte-for-byte stable on consecutive runs", () => {
  execFileSync(process.execPath, ["scripts/generate-emulator.mjs"], {
    cwd: repositoryRoot,
    stdio: "inherit",
  });
  const firstRun = generatedArtifactBytes();

  execFileSync(process.execPath, ["scripts/generate-emulator.mjs"], {
    cwd: repositoryRoot,
    stdio: "inherit",
  });
  const secondRun = generatedArtifactBytes();

  assert.deepEqual(secondRun, firstRun);
});
