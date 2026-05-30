// @vitest-environment node

import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  buildTimeoutMs,
  browserScenarioTimeoutMs,
  defaultFactorySessionID,
  exportCoverImagePath,
  expectNoBrowserErrors,
  loadReplayLines,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import { buildReplayCoverageReport } from "../src/testing/replay-fixture-catalog";

const exportFactoryDefinition = {
  inputTypes: [
    {
      name: "Factory request",
      type: "DEFAULT",
    },
  ],
  name: "Browser Export Factory",
  workers: [
    {
      body: "Return the request unchanged.",
      model: "gpt-5.4-mini",
      modelProvider: "CODEX",
      name: "browser-export-worker",
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
      name: "Browser export workstation",
      outputs: [
        {
          state: "done",
          workType: "request",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "browser-export-worker",
    },
  ],
};

const editableGraphFactoryDefinition = {
  metadata: {
    owner: "operations",
  },
  name: "Current Factory",
  resources: [
    {
      capacity: 2,
      name: "gpu",
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
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
      body: "Draft the story.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "draft",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      resources: [
        {
          capacity: 2,
          name: "gpu",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

const editableGraphFactoryReplayLines = [
  JSON.stringify({
    context: {
      eventTime: "2026-05-19T15:00:00Z",
      sequence: 1,
      tick: 1,
    },
    id: "editable-graph-1",
    payload: {
      factory: {
        resources: [
          {
            capacity: 2,
            name: "gpu",
          },
        ],
        workTypes: [
          {
            name: "story",
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
        workers: [
          {
            model: "gpt-5",
            name: "writer",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [
          {
            inputs: [
              {
                state: "queued",
                workType: "story",
              },
            ],
            name: "draft",
            outputs: [
              {
                state: "done",
                workType: "story",
              },
            ],
            resources: [
              {
                capacity: 2,
                name: "gpu",
              },
            ],
            worker: "writer",
          },
        ],
      },
    },
    type: "INITIAL_STRUCTURE_REQUEST",
  }),
  JSON.stringify({
    context: {
      eventTime: "2026-05-19T15:00:01Z",
      sequence: 2,
      tick: 2,
    },
    id: "editable-graph-2",
    payload: {
      previousState: "RUNNING",
      reason: "fixture ready",
      state: "FINISHED",
    },
    type: "FACTORY_STATE_RESPONSE",
  }),
];

describe.sequential("factory graph editor browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "enters graph editor mode through the graph card controls",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: exportFactoryDefinition,
        eventLines: await loadReplayLines("graph-state-smoke-replay.jsonl"),
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await server.replayCompleted;
        await browserPage.page
          .getByRole("button", { name: "Enter factory graph editor" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("button", { name: "Enter factory graph editor" })
          .click();

        const toolbar = browserPage.page.getByRole("region", {
          name: "Factory graph editor tools",
        });
        await toolbar.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await toolbar
          .getByRole("button", { name: "Open add entity menu" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await toolbar
          .getByText("No draft changes", { exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

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
    "exports the current factory as a downloadable PNG without uncaught browser exceptions",
    async () => {
      const postFactoryActivations = [];
      const sessionFactoryPutRequests = [];
      const replayCoverageReport = buildReplayCoverageReport();
      const pngCoverageScenario = replayCoverageReport.scenarios.find(
        (scenario) => scenario.id === "pngRoundTrip",
      );
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: exportFactoryDefinition,
        eventLines: await loadReplayLines("graph-state-smoke-replay.jsonl"),
        onActivateFactory: async (value) => {
          postFactoryActivations.push(value);
        },
        onSaveCurrentFactory: async ({ body, sessionID }) => {
          sessionFactoryPutRequests.push({ body, sessionID });
        },
      });
      const browserPage = await openBrowserPage({ acceptDownloads: true });
      const downloadDirectory = await mkdtemp(
        path.join(os.tmpdir(), "agent-factory-export-"),
      );

      expect(pngCoverageScenario).toEqual({
        description:
          "Browser export/import PNG roundtrip smoke layered on top of existing jsdom and unit PNG coverage.",
        fileName: "graph-state-smoke-replay.jsonl",
        id: "pngRoundTrip",
        surfaces: ["png-export", "png-import-preview", "png-import-activation"],
        verificationLayers: ["browser-integration", "jsdom", "unit"],
      });

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
        await browserPage.page.getByRole("button", { name: "Export PNG" }).waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        await browserPage.page.getByRole("button", { name: "Export PNG" }).click();
        await browserPage.page.getByRole("heading", { name: "Export factory" }).waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        const exportDialog = browserPage.page.getByRole("dialog", {
          name: "Export factory",
        });
        await exportDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await exportDialog
          .getByText(
            "Confirming export keeps the current dashboard state unchanged and downloads a PNG artifact with embedded you-agent-factory factory metadata.",
          )
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });

        const exportName = "Roundtrip Browser Export";
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

        expect(download.filename).toBe("roundtrip-browser-export.png");
        await exportDialog
          .getByText(
            "Downloaded roundtrip-browser-export.png. You can close this dialog or export another PNG with a different name or cover image.",
          )
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });
        await exportDialog
          .getByRole("button", { exact: true, name: "Close" })
          .click();
        await browserPage.page.getByRole("heading", { name: "Export factory" }).waitFor({
          state: "hidden",
          timeout: uiInteractionTimeoutMs,
        });
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );

        const exportedBytes = await readFile(downloadPath);
        expect(exportedBytes.subarray(0, 8)).toEqual(
          Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
        );
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
        expect(await importDialog.textContent()).toContain(
          "Choose whether to replace the factory already current in this session or create a new named factory from the embedded PNG definition.",
        );

        await importDialog
          .getByRole("button", { name: "Activate factory" })
          .click();
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
        expect(sessionFactoryPutRequests[0]?.sessionID).toBe(
          defaultFactorySessionID,
        );
        expect(sessionFactoryPutRequests[0]?.body).toMatchObject({
          ...exportFactoryDefinition,
          name: exportFactoryDefinition.name,
          version: {
            logical: "2",
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

  it(
    "creates a workstation, links it through labeled graph anchors, and saves the topology payload",
    async () => {
      const saveRequests = [];
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
        onSaveCurrentFactory: async (value) => {
          saveRequests.push(value);
        },
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await server.replayCompleted;

        await browserPage.page
          .getByRole("button", { name: "Enter factory graph editor" })
          .click();

        const toolbar = browserPage.page.getByRole("region", {
          name: "Factory graph editor tools",
        });
        await toolbar.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        await toolbar
          .getByRole("button", { name: "Open add entity menu" })
          .click();
        await browserPage.page
          .getByLabel("Add graph entity menu")
          .getByRole("button", { name: "Workstation" })
          .click();

        const addDialog = browserPage.page.getByRole("dialog", {
          name: "Add workstation",
        });
        await addDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await addDialog.getByLabel("Identifier").fill("review");
        await addDialog.getByLabel("Prompt body").fill("Review the drafted story.");
        await addDialog
          .getByRole("button", { name: "Add entity" })
          .click();

        const saveChangesButton = toolbar.getByRole("button", {
          name: "Save changes",
        });
        await expect
          .poll(async () => await saveChangesButton.isEnabled(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(true);

        await toolbar.getByRole("button", { name: "Connect" }).click();
        await browserPage.page
          .getByTestId("rf__node-workstation:draft")
          .getByRole("button", {
            name: "Route a failure transition from this workstation.",
          })
          .click();
        await browserPage.page
          .getByTestId("rf__node-work-state:story:queued")
          .getByRole("button", {
            name: "Receive workstation output into this work state.",
          })
          .click();

        await expect
          .poll(
            async () => await saveChangesButton.isEnabled(),
            {
              timeout: uiInteractionTimeoutMs,
            },
          )
          .toBe(true);

        await saveChangesButton.focus();
        await saveChangesButton.press("Enter");
        const saveDialog = browserPage.page.getByRole("dialog", {
          name: "Save factory graph changes?",
        });
        await saveDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await saveDialog
          .getByText(
            "This save will apply 1 created entity and 1 changed edge.",
            { exact: true },
          )
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await saveDialog
          .getByRole("button", { name: "Save topology" })
          .click();

        await expect
          .poll(() => saveRequests.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(1);
        await browserPage.page
          .getByText("Topology saved", { exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        expect(saveRequests).toEqual([
          {
            body: {
              ...editableGraphFactoryDefinition,
              version: {
                logical: "2",
                physical: "2026-05-19T00:00:00.001Z",
              },
              workstations: [
                {
                  ...editableGraphFactoryDefinition.workstations[0],
                  onFailure: [
                    {
                      state: "queued",
                      workType: "story",
                    },
                  ],
                },
                {
                  body: "Review the drafted story.",
                  inputs: [],
                  name: "review",
                  type: "MODEL_WORKSTATION",
                  worker: "writer",
                },
              ],
            },
            mode: "REPLACE_CURRENT",
            sessionID: "~default",
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
