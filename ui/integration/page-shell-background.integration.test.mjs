// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
} from "./browser-test-harness.mjs";

const pageShellFactoryDefinition = {
  name: "Page Shell Harness",
};

describe.sequential("page-shell background browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  }, buildTimeoutMs);

  it(
    "applies a flat foundation blue page shell on the document root",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: pageShellFactoryDefinition,
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "page-shell-background",
      });

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });

        const shell = await browserPage.page.evaluate(() => {
          const root = getComputedStyle(document.documentElement);
          return {
            backgroundColor: root.backgroundColor,
            backgroundImage: root.backgroundImage,
          };
        });

        expect(
          shell.backgroundImage === "none" || shell.backgroundImage === "",
        ).toBe(true);
        expect(shell.backgroundColor).toBe("rgb(10, 17, 23)");
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
