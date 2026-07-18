import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import process from "node:process";
import test from "node:test";

const repositoryRoot = resolve(import.meta.dirname, "../../..");
const authoredSchemaPath = join(
  repositoryRoot,
  "contracts/factory-emulator/scenario.schema.json",
);
const generatedTypesPath = join(
  repositoryRoot,
  "packages/factory-emulator/src/generated/scenario.ts",
);
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

test("TypeScript declarations are derived from the complete authored schema", () => {
  const originalSchema = readFileSync(authoredSchemaPath);
  const changedSchema = JSON.parse(originalSchema);
  changedSchema.required.push("activityLabel");
  changedSchema.properties.seed.maxLength = 257;
  changedSchema.$defs.matcher.oneOf.push({
    type: "object",
    additionalProperties: false,
    required: ["kind", "probe"],
    properties: {
      kind: { const: "generationProbe" },
      probe: { type: "boolean" },
    },
  });

  try {
    writeFileSync(authoredSchemaPath, `${JSON.stringify(changedSchema, null, 2)}\n`);
    assert.equal(runGeneration().status, 0);

    const generatedTypes = readFileSync(generatedTypesPath, "utf8");
    assert.match(generatedTypes, /Maximum length: 257\./);
    assert.match(generatedTypes, /kind: "generationProbe";/);
    assert.match(generatedTypes, /probe: boolean;/);
    assert.match(generatedTypes, /activityLabel: string;/);
    assert.doesNotMatch(generatedTypes, /activityLabel\?: string;/);
  } finally {
    writeFileSync(authoredSchemaPath, originalSchema);
    assert.equal(runGeneration().status, 0);
  }
});
