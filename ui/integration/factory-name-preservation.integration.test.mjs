// @vitest-environment node
// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: browser name-preservation scenarios share one preview harness and sequential server lifecycle.

import { rm } from "node:fs/promises";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  openBrowserPage,
  resolvedDefaultFactorySessionID,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import {
  canonicalSessionFactoryName,
  editableGraphFactoryDefinition,
  editableGraphFactoryReplayLines,
  exportFactoryDefinition,
  exportFactoryPngFromDashboard,
  exportFactoryReplayLines,
  installCapturedDownloadHook,
  openImportDialogForDroppedPng,
  saveGraphEditorTopology,
  waitForDashboardReady,
} from "./factory-name-preservation-browser-helpers.mjs";

describe.sequential("factory name preservation browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "keeps the canonical session factory name on replace-current dashboard saves",
    async () => {
      const saveRequests = [];
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
        onSaveCurrentFactory: async (request) => {
          saveRequests.push(request);
        },
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "factory-name-preservation-save",
      });

      try {
        await waitForDashboardReady(
          browserPage.page,
          preview.previewURL,
          server,
        );
        await browserPage.page
          .getByRole("button", { name: "Edit mode" })
          .click();
        await saveGraphEditorTopology(browserPage.page);

        await expect
          .poll(() => saveRequests.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(1);

        expect(saveRequests[0]?.sessionID).toBe(
          resolvedDefaultFactorySessionID,
        );
        expect(saveRequests[0]?.mode).toBe("REPLACE_CURRENT");
        expect(saveRequests[0]?.body?.name).toBe(canonicalSessionFactoryName);

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

  it(
    "keeps the current session factory name on replace-current import activation",
    async () => {
      const sessionFactoryPutRequests = [];
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: exportFactoryDefinition,
        eventLines: exportFactoryReplayLines,
        onSaveCurrentFactory: async (request) => {
          sessionFactoryPutRequests.push(request);
        },
      });
      const browserPage = await openBrowserPage({
        acceptDownloads: true,
        artifactLabel: "factory-name-preservation-import-replace",
      });
      let downloadDirectory = null;

      await installCapturedDownloadHook(browserPage.page);

      try {
        await waitForDashboardReady(
          browserPage.page,
          preview.previewURL,
          server,
        );

        const embeddedImportName = "Dropped Import Label";
        const exportResult = await exportFactoryPngFromDashboard(
          browserPage.page,
          embeddedImportName,
        );
        downloadDirectory = exportResult.downloadDirectory;

        const importDialog = await openImportDialogForDroppedPng(
          browserPage.page,
          exportResult.downloadPath,
          exportResult.download,
        );
        await importDialog
          .getByRole("img", { name: `${embeddedImportName} preview` })
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });

        await importDialog
          .getByRole("button", { name: "Confirm import" })
          .click();
        const importOutcome = await Promise.race([
          (async () => {
            await expect
              .poll(async () => sessionFactoryPutRequests.length, {
                timeout: uiInteractionTimeoutMs,
              })
              .toBe(1);
            return "request";
          })(),
          importDialog
            .getByRole("alert")
            .waitFor({
              state: "visible",
              timeout: uiInteractionTimeoutMs,
            })
            .then(() => "error"),
        ]);
        if (importOutcome === "error") {
          throw new Error(await importDialog.getByRole("alert").innerText());
        }
        await importDialog.waitFor({
          state: "hidden",
          timeout: uiInteractionTimeoutMs,
        });

        expect(sessionFactoryPutRequests[0]?.sessionID).toBe(
          resolvedDefaultFactorySessionID,
        );
        expect(sessionFactoryPutRequests[0]?.mode).toBe("REPLACE_CURRENT");
        expect(sessionFactoryPutRequests[0]?.body?.name).toBe(
          exportFactoryDefinition.name,
        );
        expect(sessionFactoryPutRequests[0]?.body?.name).not.toBe(
          embeddedImportName,
        );

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        if (downloadDirectory) {
          await rm(downloadDirectory, { force: true, recursive: true });
        }
        await server.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "persists the chosen create-new-named target on import activation",
    async () => {
      const sessionFactoryPutRequests = [];
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: exportFactoryDefinition,
        eventLines: exportFactoryReplayLines,
        onSaveCurrentFactory: async (request) => {
          sessionFactoryPutRequests.push(request);
        },
      });
      const browserPage = await openBrowserPage({
        acceptDownloads: true,
        artifactLabel: "factory-name-preservation-import-create-named",
      });
      let downloadDirectory = null;

      await installCapturedDownloadHook(browserPage.page);

      try {
        await waitForDashboardReady(
          browserPage.page,
          preview.previewURL,
          server,
        );

        const embeddedImportName = "New Named Import Target";
        const exportResult = await exportFactoryPngFromDashboard(
          browserPage.page,
          embeddedImportName,
        );
        downloadDirectory = exportResult.downloadDirectory;

        const importDialog = await openImportDialogForDroppedPng(
          browserPage.page,
          exportResult.downloadPath,
          exportResult.download,
        );
        await importDialog
          .getByRole("img", { name: `${embeddedImportName} preview` })
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });

        await importDialog
          .getByRole("radiogroup", { name: "Import save choice" })
          .getByRole("radio", { name: "Create new named factory" })
          .click();
        await expect
          .poll(
            async () =>
              await importDialog
                .getByRole("radiogroup", { name: "Import save choice" })
                .getByRole("radio", { name: "Create new named factory" })
                .isChecked(),
            {
              timeout: uiInteractionTimeoutMs,
            },
          )
          .toBe(true);
        const resolvedCreateName = await importDialog
          .getByText("New factory name", { exact: true })
          .locator("xpath=following-sibling::span[1]")
          .innerText();

        await importDialog
          .getByRole("button", { name: "Confirm import" })
          .click();
        const importOutcome = await Promise.race([
          (async () => {
            await expect
              .poll(async () => sessionFactoryPutRequests.length, {
                timeout: uiInteractionTimeoutMs,
              })
              .toBe(1);
            return "request";
          })(),
          importDialog
            .getByRole("alert")
            .waitFor({
              state: "visible",
              timeout: uiInteractionTimeoutMs,
            })
            .then(() => "error"),
        ]);
        if (importOutcome === "error") {
          throw new Error(await importDialog.getByRole("alert").innerText());
        }
        await importDialog.waitFor({
          state: "hidden",
          timeout: uiInteractionTimeoutMs,
        });

        expect(sessionFactoryPutRequests[0]?.sessionID).toBe(
          resolvedDefaultFactorySessionID,
        );
        expect(sessionFactoryPutRequests[0]?.mode).toBe(
          "UPSERT_NAMED_AND_ACTIVATE",
        );
        expect(sessionFactoryPutRequests[0]?.body?.name).toBe(
          resolvedCreateName,
        );
        expect(sessionFactoryPutRequests[0]?.body?.name).not.toBe(
          exportFactoryDefinition.name,
        );

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        if (downloadDirectory) {
          await rm(downloadDirectory, { force: true, recursive: true });
        }
        await server.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );
});
