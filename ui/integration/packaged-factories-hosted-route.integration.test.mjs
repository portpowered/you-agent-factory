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

const catalogResponse = {
  factories: [
    {
      json: { id: "builtin-example", name: "example" },
      name: "@you/example",
      project: "builtin-example",
      slug: "example",
      yaml: "id: builtin-example\nname: example\n",
    },
  ],
};

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
          // The Vite preview serves the dashboard only. Fulfill the backend
          // contract here; its handler response is verified in Go.
          await browserPage.page.route("**/packaged-factories", (route) => {
            if (new URL(route.request().url()).pathname !== "/packaged-factories") {
              return route.continue();
            }
            return route.fulfill({
              body: JSON.stringify(catalogResponse),
              contentType: "application/json",
            });
          });
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
