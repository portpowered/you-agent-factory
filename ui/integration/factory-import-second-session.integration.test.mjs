// @vitest-environment node

import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  buildTimeoutMs,
  browserScenarioTimeoutMs,
  exportCoverImagePath,
  expectNoBrowserErrors,
  loadReplayLines,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

const nonDefaultSessionID = "session-review";

const defaultFactoryDefinition = {
  name: "Browser Session Harness Factory",
};

const reviewSessionFactoryDefinition = {
  inputTypes: [
    {
      name: "Factory request",
      type: "DEFAULT",
    },
  ],
  name: "Review Session Import Factory",
  workers: [
    {
      body: "Return the request unchanged.",
      model: "gpt-5.4-mini",
      modelProvider: "CODEX",
      name: "review-import-worker",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "request",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  workstations: [
    {
      behavior: "STANDARD",
      inputs: [
        {
          state: "queued",
          workType: "request",
        },
      ],
      name: "Review import workstation",
      outputs: [
        {
          state: "done",
          workType: "request",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "review-import-worker",
    },
  ],
};

function renameReplayWorkstation(lines, workstationName) {
  return lines.map((line) => {
    const event = JSON.parse(line);
    const payloadFactory = event.payload?.factory;
    if (Array.isArray(payloadFactory?.workstations)) {
      payloadFactory.workstations = payloadFactory.workstations.map(
        (workstation) => ({
          ...workstation,
          name: workstationName,
        }),
      );
    }

    const payloadWorkstation = event.payload?.workstation;
    if (payloadWorkstation && typeof payloadWorkstation === "object") {
      event.payload.workstation = {
        ...payloadWorkstation,
        name: workstationName,
      };
    }

    return JSON.stringify(event);
  });
}

async function openReviewSessionTab(page) {
  await page
    .getByRole("button", { name: "Open another session" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await page.getByRole("button", { name: "Open another session" }).click();

  const dialog = page.getByRole("dialog", {
    name: "Factory Session",
  });
  await dialog.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await dialog.getByLabel("Factory folder").fill("/workspace/project");
  await dialog.getByRole("button", { name: "Start Factory" }).click();
  await dialog
    .getByRole("button", { name: "Review factory" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await dialog.getByRole("button", { name: "Review factory" }).click();
  await dialog.waitFor({
    state: "hidden",
    timeout: uiInteractionTimeoutMs,
  });

  const activeReviewTab = page.getByRole("tab", {
    name: "review",
    selected: true,
  });
  await activeReviewTab.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
}

describe.sequential("factory import second session browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "activates imported factory through PUT on the selected non-default session tab without POST /factories",
    async () => {
      const postFactoryActivations = [];
      const sessionFactoryPutRequests = [];
      const sessionReplayLines = renameReplayWorkstation(
        await loadReplayLines("graph-state-smoke-replay.jsonl"),
        "Session Review",
      );
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: await loadReplayLines("graph-state-smoke-replay.jsonl"),
        eventLinesBySessionID: {
          [nonDefaultSessionID]: sessionReplayLines,
        },
        currentFactoryBySessionID: {
          [nonDefaultSessionID]: reviewSessionFactoryDefinition,
        },
        onActivateFactory: async (body) => {
          postFactoryActivations.push(body);
        },
        onSaveCurrentFactory: async ({ body, sessionID }) => {
          sessionFactoryPutRequests.push({ body, sessionID });
        },
        onOpenFactorySession: async (body) => {
          if (!body?.target) {
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
          }

          return {
            session: {
              factoryDir: "/workspace/project/review",
              folderPath: "/workspace/project",
              id: nonDefaultSessionID,
              isDefault: false,
              project: "review",
              target: {
                kind: "named",
                name: "review",
              },
            },
          };
        },
      });
      const browserPage = await openBrowserPage({
        acceptDownloads: true,
        artifactLabel: "factory-import-second-session",
      });
      const downloadDirectory = await mkdtemp(
        path.join(os.tmpdir(), "agent-factory-import-second-session-"),
      );

      await browserPage.page.addInitScript(() => {
        window.__agentFactoryCapturedDownloads = [];
        const originalClick = HTMLAnchorElement.prototype.click;
        HTMLAnchorElement.prototype.click = function click(...args) {
          if (this.download && this.href.startsWith("blob:")) {
            const filename = this.download;
            const href = this.href;
            const capture = fetch(href)
              .then(async (response) => {
                const buffer = await response.arrayBuffer();
                return {
                  bytes: Array.from(new Uint8Array(buffer)),
                  filename,
                };
              })
              .then((download) => {
                window.__agentFactoryCapturedDownloads.push(download);
              });
            window.__agentFactoryPendingDownload = capture;
          }

          return originalClick.apply(this, args);
        };
      });

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", { level: 1, name: "U", exact: true })
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });
        await server.replayCompleted;

        await openReviewSessionTab(browserPage.page);
        await expect
          .poll(
            async () => server.requestedEventSessionIDs.includes(nonDefaultSessionID),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe(true);
        await browserPage.page
          .getByRole("button", { name: "Select Session Review" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        await browserPage.page.getByRole("button", { name: "Export PNG" }).waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await browserPage.page.getByRole("button", { name: "Export PNG" }).click();

        const exportDialog = browserPage.page.getByRole("dialog", {
          name: "Export factory",
        });
        await exportDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        const exportName = "Second Tab Import Roundtrip";
        await exportDialog.getByLabel("Factory name").fill(exportName);
        await exportDialog
          .getByLabel("Cover image")
          .setInputFiles(exportCoverImagePath);
        await exportDialog.getByText("Selected image: dashboard.png").waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        const exportDialogButton = exportDialog.getByRole("button", {
          name: "Export PNG",
        });
        await expect
          .poll(async () => await exportDialogButton.isEnabled(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(true);
        await exportDialogButton.click();

        const exportOutcome = await Promise.race([
          browserPage.page
            .waitForFunction(
              () => window.__agentFactoryCapturedDownloads.length > 0,
              null,
              { timeout: uiInteractionTimeoutMs },
            )
            .then(() => "download"),
          exportDialog
            .getByRole("alert")
            .waitFor({
              state: "visible",
              timeout: uiInteractionTimeoutMs,
            })
            .then(() => "error"),
        ]);
        if (exportOutcome === "error") {
          throw new Error(await exportDialog.getByRole("alert").innerText());
        }

        const download = await browserPage.page.evaluate(
          () => window.__agentFactoryCapturedDownloads[0] ?? null,
        );
        expect(download).not.toBeNull();
        const downloadPath = path.join(downloadDirectory, download.filename);
        await writeFile(downloadPath, new Uint8Array(download.bytes));
        await exportDialog
          .getByRole("button", { exact: true, name: "Close" })
          .click();
        await browserPage.page.getByRole("heading", { name: "Export factory" }).waitFor({
          state: "hidden",
          timeout: uiInteractionTimeoutMs,
        });

        const exportedBytes = await readFile(downloadPath);
        const viewport = browserPage.page.getByRole("region", {
          name: "Work graph viewport",
        });
        const importDataTransfer = await browserPage.page.evaluateHandle(
          ({ bytes, fileName }) => {
            const dataTransfer = new DataTransfer();
            dataTransfer.items.add(
              new File([new Uint8Array(bytes)], fileName, { type: "image/png" }),
            );
            return dataTransfer;
          },
          {
            bytes: Array.from(exportedBytes),
            fileName: download.filename,
          },
        );

        await viewport.dispatchEvent("dragover", {
          dataTransfer: importDataTransfer,
        });
        await browserPage.page.getByText("Import factory PNG").waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await viewport.dispatchEvent("drop", { dataTransfer: importDataTransfer });

        const importDialog = browserPage.page.getByRole("dialog", {
          name: "Review factory import",
        });
        await importDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await importDialog
          .getByRole("img", { name: `${exportName} preview` })
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });
        expect(await importDialog.textContent()).toContain(exportName);
        expect(await importDialog.textContent()).toContain(download.filename);

        const activateButton = importDialog.getByRole("button", {
          name: "Activate factory",
        });
        await expect
          .poll(async () => await activateButton.isEnabled(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(true);
        await activateButton.click();
        await expect
          .poll(async () => sessionFactoryPutRequests.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(1);
        await importDialog.waitFor({
          state: "hidden",
          timeout: uiInteractionTimeoutMs,
        });

        expect(postFactoryActivations).toEqual([]);
        expect(sessionFactoryPutRequests).toHaveLength(1);
        expect(sessionFactoryPutRequests[0]?.sessionID).toBe(nonDefaultSessionID);
        expect(sessionFactoryPutRequests[0]?.body).toMatchObject({
          ...reviewSessionFactoryDefinition,
          name: reviewSessionFactoryDefinition.name,
          version: {
            logical: "2",
            physical: "2026-05-19T00:00:00.001Z",
          },
        });
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await rm(downloadDirectory, { force: true, recursive: true });
        await server.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );
});
