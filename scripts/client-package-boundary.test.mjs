import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const packageRoot = new URL("../packages/client/", import.meta.url);

test("client package remains framework and host neutral", async () => {
  const packageJson = JSON.parse(
    await readFile(new URL("package.json", packageRoot), "utf8"),
  );
  assert.deepEqual(Object.keys(packageJson.dependencies ?? {}).sort(), [
    "ajv",
    "ajv-formats",
  ]);

  const publicSources = (
    await Promise.all(
      [
        "src/contracts.ts",
        "src/index.ts",
        "src/index.js",
        "src/recording-parser.d.ts",
        "src/recording-parser.js",
      ].map((path) => readFile(new URL(path, packageRoot), "utf8")),
    )
  ).join("\n");

  for (const prohibited of [
    "react",
    "zustand",
    "dashboard",
    "localStorage",
    "sessionStorage",
    "react-router",
    "ApiError",
  ]) {
    assert.doesNotMatch(publicSources, new RegExp(prohibited, "i"));
  }
});
