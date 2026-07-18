import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { appendFile, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../", import.meta.url));
const generatedPaths = [
  join(repositoryRoot, "packages", "client", "src", "generated", "openapi.ts"),
  join(
    repositoryRoot,
    "packages",
    "client",
    "src",
    "generated",
    "factory-recording.schema.json",
  ),
];

function checkFreshness() {
  return spawnSync(
    process.execPath,
    ["scripts/generate-client.mjs", "--check"],
    {
      cwd: repositoryRoot,
      encoding: "utf8",
    },
  );
}

test("freshness verification rejects drift in every generated client surface", async () => {
  for (const generatedPath of generatedPaths) {
    const original = await readFile(generatedPath);
    try {
      await appendFile(generatedPath, "\n// deliberate freshness-test drift\n");
      const result = checkFreshness();
      assert.notEqual(
        result.status,
        0,
        `${generatedPath} was incorrectly fresh`,
      );
      assert.match(
        `${result.stdout}\n${result.stderr}`,
        /Generated (?:OpenAPI types are|Factory Recording schema is) stale/,
      );
    } finally {
      await writeFile(generatedPath, original);
    }
  }

  const result = checkFreshness();
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
});
