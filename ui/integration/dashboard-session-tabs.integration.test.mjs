// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  buildTimeoutMs,
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  loadReplayLines,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

const defaultFactoryDefinition = {
  name: "Browser Session Harness Factory",
};

describe.sequential("dashboard session tabs browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "opens the session dialog and lists runnable targets from folder inspection",
    async () => {
      const openSessionRequests = [];
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: await loadReplayLines("graph-state-smoke-replay.jsonl"),
        onOpenFactorySession: async (body) => {
          openSessionRequests.push(body);
          return {
            targets: [
              {
                factoryDir: "/workspace/project/review",
                folderPath: "/workspace/project",
                label: "Review factory",
                project: "review",
                ref: {
                  kind: "named",
                  name: "review",
                },
              },
            ],
          };
        },
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("button", { name: "Open another session" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("button", { name: "Open another session" })
          .click();

        const dialog = browserPage.page.getByRole("dialog", {
          name: "Open factory session",
        });
        await dialog.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await dialog.getByLabel("Factory folder").fill("/workspace/project");
        await dialog.getByRole("button", { name: "Inspect folder" }).click();

        await dialog
          .getByRole("region", { name: "Pick a runnable target" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await dialog
          .getByText("Choose one runnable target from this folder.")
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await dialog
          .getByRole("button", { name: /Review factory/ })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        expect(openSessionRequests).toEqual([
          {
            folderPath: "/workspace/project",
          },
        ]);
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await server.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );
});
