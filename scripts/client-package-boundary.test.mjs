import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const packageRoot = new URL("../packages/client/", import.meta.url);

test("client package remains framework and host neutral", async () => {
  const packageJson = JSON.parse(
    await readFile(new URL("package.json", packageRoot), "utf8"),
  );
  assert.deepEqual(packageJson.dependencies ?? {}, {});

  const contracts = await readFile(
    new URL("src/contracts.ts", packageRoot),
    "utf8",
  );
  const publicIndex = await readFile(
    new URL("src/index.ts", packageRoot),
    "utf8",
  );
  const publicSources = `${contracts}\n${publicIndex}`;

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
