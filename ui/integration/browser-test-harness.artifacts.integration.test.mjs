// @vitest-environment node

import path from "node:path";
import { fileURLToPath } from "node:url";

import { afterEach, describe, expect, it } from "vitest";

import { browserArtifactDirectory } from "./browser-test-harness.mjs";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const originalArtifactDirectory = process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;

afterEach(() => {
  if (originalArtifactDirectory === undefined) {
    delete process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;
    return;
  }
  process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR = originalArtifactDirectory;
});

describe("browser artifact directory", () => {
  it("resolves workflow-provided repo artifact paths from the UI package root", () => {
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR =
      "../.artifacts/ui-browser-integration/browser";

    expect(browserArtifactDirectory()).toBe(
      path.resolve(
        packageRoot,
        "..",
        ".artifacts",
        "ui-browser-integration",
        "browser",
      ),
    );
  });
});
