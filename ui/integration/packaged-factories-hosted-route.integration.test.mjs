// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  openBrowserPage,
  startBrowserPreview,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

const viewports = [
  { height: 844, width: 390 },
  { height: 1024, width: 768 },
  { height: 900, width: 1440 },
];

describe.sequential("Packaged Factories production route", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "renders the catalog without page overflow at the hosted dashboard path",
    async () => {
      for (const viewport of viewports) {
        const browserPage = await openBrowserPage({
          artifactLabel: `packaged-factories-${viewport.width}x${viewport.height}`,
        });

        try {
          await browserPage.page.setViewportSize(viewport);
          await browserPage.page.goto(
            new URL("packaged-factories", preview.previewURL).href,
            { waitUntil: "domcontentloaded" },
          );
          await browserPage.page
            .getByRole("heading", { level: 2, name: "Packaged Factories" })
            .waitFor({
              state: "visible",
              timeout: uiInteractionTimeoutMs,
            });

          expect(
            await browserPage.page.evaluate(
              () =>
                document.documentElement.scrollWidth <=
                document.documentElement.clientWidth,
            ),
          ).toBe(true);
          expectNoBrowserErrors(
            browserPage.pageErrors,
            browserPage.consoleErrors,
            expect,
          );
        } finally {
          await browserPage.close();
        }
      }
    },
    browserScenarioTimeoutMs,
  );
});
