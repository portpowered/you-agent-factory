// @vitest-environment node

import { mkdtemp, readdir, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { afterEach, describe, expect, it } from "vitest";

import {
  browserArtifactDirectory,
  installBrowserErrorCapture,
  openBrowserPage,
} from "./browser-test-harness.mjs";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const originalArtifactDirectory =
  process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;
const originalArtifactWorkerIsolation =
  process.env.AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION;

afterEach(() => {
  if (originalArtifactDirectory === undefined) {
    delete process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;
  } else {
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR = originalArtifactDirectory;
  }
  if (originalArtifactWorkerIsolation === undefined) {
    delete process.env.AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION;
  } else {
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION =
      originalArtifactWorkerIsolation;
  }
});

describe("browser artifact directory", () => {
  it("resolves workflow-provided repo artifact paths from the UI package root", () => {
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR =
      "../.artifacts/ui-browser-integration/browser";
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION = "false";

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

  it("isolates workflow artifacts by Vitest worker", () => {
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR =
      "../.artifacts/ui-browser-integration/browser";
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION = "true";
    const workerID =
      process.env.VITEST_POOL_ID ??
      process.env.VITEST_WORKER_ID ??
      String(process.pid);

    expect(browserArtifactDirectory()).toBe(
      path.resolve(
        packageRoot,
        "..",
        ".artifacts",
        "ui-browser-integration",
        "browser",
        `worker-${workerID}`,
      ),
    );
  });

  it("emits only bounded diagnostic counts in bounded artifact mode", async () => {
    const artifactDirectory = await mkdtemp(
      path.join(os.tmpdir(), "you-browser-artifacts-"),
    );
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR = artifactDirectory;
    process.env.AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION = "true";
    const isolatedArtifactDirectory = browserArtifactDirectory();
    let browserPage = null;

    try {
      browserPage = await openBrowserPage({
        artifactLabel: "bounded-artifacts",
        artifactMode: "bounded",
        diagnosticCharacterLimit: 12,
        diagnosticLimit: 2,
      });
      for (let index = 0; index < 3; index += 1) {
        await browserPage.page.evaluate((errorIndex) => {
          console.error(`sensitive-payload-${errorIndex}`);
        }, index);
      }
      await expect.poll(() => browserPage.consoleErrors.length).toBe(2);
      expect(
        browserPage.consoleErrors.every((error) => error.length <= 12),
      ).toBe(true);

      await browserPage.close();
      browserPage = null;

      expect(await readdir(isolatedArtifactDirectory)).toEqual([
        "bounded-artifacts.diagnostics.json",
      ]);
      const diagnostics = await readFile(
        path.join(
          isolatedArtifactDirectory,
          "bounded-artifacts.diagnostics.json",
        ),
        "utf8",
      );
      expect(diagnostics).not.toContain("sensitive-payload");
      expect(JSON.parse(diagnostics)).toMatchObject({
        artifactLabel: "bounded-artifacts",
        consoleErrorCount: 2,
        pageErrorCount: 0,
      });
    } finally {
      await browserPage?.close().catch(() => {});
      await rm(artifactDirectory, { force: true, recursive: true });
    }
  });

  it("bounds diagnostic entries from a second page by count and length", async () => {
    let browserPage = null;
    let secondPage = null;

    try {
      browserPage = await openBrowserPage({ artifactMode: "bounded" });
      secondPage = await browserPage.context.newPage();
      const secondPageErrors = installBrowserErrorCapture(secondPage, {
        characterLimit: 12,
        entryLimit: 2,
      });

      for (let index = 0; index < 3; index += 1) {
        await secondPage.evaluate((errorIndex) => {
          console.error(`sensitive-second-tab-payload-${errorIndex}`);
        }, index);
      }

      await expect.poll(() => secondPageErrors.consoleErrors.length).toBe(2);
      expect(
        secondPageErrors.consoleErrors.every((error) => error.length <= 12),
      ).toBe(true);
      expect(secondPageErrors.consoleErrors.join(" ")).not.toContain("payload");
    } finally {
      await secondPage?.close().catch(() => {});
      await browserPage?.close().catch(() => {});
    }
  });
});
