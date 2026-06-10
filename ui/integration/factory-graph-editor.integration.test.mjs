// @vitest-environment node

import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { buildReplayCoverageReport } from "../src/testing/replay-fixture-catalog";
import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  defaultFactorySessionID,
  expectNoBrowserErrors,
  exportCoverImagePath,
  fillWorkstationPromptBody,
  loadReplayLines,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

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

function factoryGraphCardScope(page) {
  return page.getByRole("article", { name: "Factory graph" });
}

async function expectConsolidatedDirtyGraphEditorChrome(page) {
  const graphCard = factoryGraphCardScope(page);
  await expect
    .poll(
      async () => {
        const toggle = graphCard.getByRole("button", {
          name: "Leave editor",
        });
        const toggleClassName = await toggle.getAttribute("class");

        return toggleClassName?.includes("border-af-warning-border") === true
          ? 1
          : 0;
      },
      {
        timeout: uiInteractionTimeoutMs,
      },
    )
    .toBe(1);

  const toolbar = graphCard.getByRole("region", {
    name: "Factory graph editor tools",
  });
  expect(await toolbar.locator('[role="status"]').count()).toBe(0);
}

async function expectTooltipPlacement(
  scope,
  triggerName,
  tooltipName,
  placement,
) {
  const trigger = scope.getByRole("button", { name: triggerName });
  await expect
    .poll(() => trigger.isEnabled(), { timeout: uiInteractionTimeoutMs })
    .toBe(true);
  await trigger.focus();
  const tooltip = scope.getByRole("tooltip", { name: tooltipName });
  await tooltip.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const className = await tooltip.getAttribute("class");

  if (placement === "above") {
    expect(className).toContain("bottom-full");
    expect(className).not.toContain("top-full");
  } else {
    expect(className).toContain("top-full");
    expect(className).not.toContain("bottom-full");
  }

  await trigger.evaluate((element) => {
    element.blur();
  });
  await tooltip.waitFor({ state: "hidden", timeout: uiInteractionTimeoutMs });
}

async function expectConsolidatedCleanGraphEditorChrome(page) {
  const graphCard = factoryGraphCardScope(page);
  await expect
    .poll(
      async () => {
        const toggle = graphCard.getByRole("button", {
          name: "Leave editor",
        });
        const toggleClassName = await toggle.getAttribute("class");
        const toolbar = graphCard.getByRole("region", {
          name: "Factory graph editor tools",
        });
        const toolbarStatusCount = await toolbar
          .locator('[role="status"]')
          .count();

        return toggleClassName?.includes("border-af-warning-border") !== true &&
          toolbarStatusCount === 0
          ? 1
          : 0;
      },
      {
        timeout: uiInteractionTimeoutMs,
      },
    )
    .toBe(1);
}

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
    "keeps toolbar hints above the trigger and mode-toggle hints below on short viewports",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: exportFactoryDefinition,
        eventLines: await loadReplayLines("graph-state-smoke-replay.jsonl"),
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.setViewportSize({ width: 480, height: 384 });
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await server.replayCompleted;

        const graphCard = factoryGraphCardScope(browserPage.page);
        await expectTooltipPlacement(
          graphCard,
          "Edit mode",
          "Edit mode",
          "below",
        );

        await graphCard.getByRole("button", { name: "Edit mode" }).click();

        const toolbar = graphCard.getByRole("region", {
          name: "Factory graph editor tools",
        });
        await toolbar.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        for (const [triggerName, tooltipName] of [
          ["Delete", "Remove"],
          ["Reset layout", "Reset node positions to the saved shared layout baseline"],
          ["Show or hide", "Show"],
        ]) {
          await expectTooltipPlacement(
            toolbar,
            triggerName,
            tooltipName,
            "above",
          );
        }

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
          .getByRole("button", { name: "Edit mode" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("button", { name: "Edit mode" })
          .click();

        const toolbar = factoryGraphCardScope(browserPage.page).getByRole(
          "region",
          {
            name: "Factory graph editor tools",
          },
        );
        await toolbar.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await toolbar
          .getByRole("button", { name: "Add" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        expect(
          await toolbar
            .getByRole("button", { name: "Discard changes" })
            .isDisabled(),
        ).toBe(true);

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
      const sessionFactoryPutRequests = [];
      const replayCoverageReport = buildReplayCoverageReport();
      const pngCoverageScenario = replayCoverageReport.scenarios.find(
        (scenario) => scenario.id === "pngRoundTrip",
      );
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: exportFactoryDefinition,
        eventLines: await loadReplayLines("graph-state-smoke-replay.jsonl"),
        onSaveCurrentFactory: async (request) => {
          sessionFactoryPutRequests.push(request);
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
        await browserPage.page
          .getByRole("button", { name: "Export PNG" })
          .waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });

        await browserPage.page
          .getByRole("button", { name: "Export PNG" })
          .click();
        await browserPage.page
          .getByRole("heading", { name: "Export factory" })
          .waitFor({
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
        await browserPage.page
          .getByRole("heading", { name: "Export factory" })
          .waitFor({
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
              new File([new Uint8Array(bytes)], fileName, {
                type: "image/png",
              }),
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
        await viewport.dispatchEvent("drop", {
          dataTransfer: importDataTransfer,
        });

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
          "Replace current factory",
        );
        expect(await importDialog.textContent()).toContain(
          "Replace current factory keeps the session factory name and updates its definition.",
        );

        await importDialog
          .getByRole("button", { name: "Confirm import" })
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
          .getByRole("button", { name: "Edit mode" })
          .click();

        const toolbar = factoryGraphCardScope(browserPage.page).getByRole(
          "region",
          {
            name: "Factory graph editor tools",
          },
        );
        await toolbar.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        await toolbar.getByRole("button", { name: "Add" }).click();
        await browserPage.page
          .getByLabel("Add graph entity menu")
          .getByRole("button", { name: "Workstation" })
          .evaluate((button) => button.click());

        const addDialog = browserPage.page.getByRole("dialog", {
          name: "Add workstation",
        });
        await addDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await addDialog.getByLabel("Identifier").fill("review");
        await fillWorkstationPromptBody(addDialog, "Review the drafted story.");
        await addDialog.getByRole("button", { name: "Add entity" }).click();

        const saveChangesButton = toolbar.getByRole("button", {
          name: "Save changes",
        });
        await expect
          .poll(async () => await saveChangesButton.isEnabled(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(true);

        await browserPage.page
          .getByTestId("rf__node-workstation:draft")
          .getByLabel("Route successful output from this workstation.")
          .click();
        await browserPage.page
          .getByTestId("rf__node-work-state:story:queued")
          .getByLabel("Receive workstation output into this work state.")
          .click();

        await expect
          .poll(async () => await saveChangesButton.isEnabled(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(true);

        await expectConsolidatedDirtyGraphEditorChrome(browserPage.page);

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
        await saveDialog.getByRole("button", { name: "Save topology" }).click();

        await expect
          .poll(() => saveRequests.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(1);
        await browserPage.page
          .getByText("Topology saved", { exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        await expectConsolidatedCleanGraphEditorChrome(browserPage.page);
        expect(
          await toolbar
            .getByRole("button", { name: "Discard changes" })
            .isDisabled(),
        ).toBe(true);

        expect(saveRequests).toHaveLength(1);
        expect(saveRequests[0]?.sessionID).toBe("~default");
        expect(saveRequests[0]?.body).toMatchObject({
          name: editableGraphFactoryDefinition.name,
          version: {
            logical: "2",
            physical: "2026-05-19T00:00:00.001Z",
          },
          workstations: [
            {
              name: "draft",
              outputs: [
                {
                  state: "done",
                  workType: "story",
                },
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
        });
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
    "shows consolidated unsaved status chrome after a topology edit and clears it after discard",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await server.replayCompleted;

        await browserPage.page
          .getByRole("button", { name: "Edit mode" })
          .click();

        const toolbar = factoryGraphCardScope(browserPage.page).getByRole(
          "region",
          {
            name: "Factory graph editor tools",
          },
        );
        await toolbar.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        await toolbar.getByRole("button", { name: "Add" }).click();
        await browserPage.page
          .getByLabel("Add graph entity menu")
          .getByRole("button", { name: "Work type" })
          .evaluate((button) => button.click());

        const addDialog = browserPage.page.getByRole("dialog", {
          name: "Add work type",
        });
        await addDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await addDialog.getByLabel("Identifier").fill("essay");
        await addDialog.getByLabel("First state").fill("queued");
        await addDialog.getByRole("button", { name: "Add entity" }).click();

        const discardChangesButton = toolbar.getByRole("button", {
          name: "Discard changes",
        });
        await expect
          .poll(async () => await discardChangesButton.isEnabled(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(true);

        await expectConsolidatedDirtyGraphEditorChrome(browserPage.page);

        await discardChangesButton.click();
        await expectConsolidatedCleanGraphEditorChrome(browserPage.page);
        expect(
          await toolbar
            .getByRole("button", { name: "Discard changes" })
            .isDisabled(),
        ).toBe(true);

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
    "discards pending graph edits and leaves the factory graph editor without saving",
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
          .getByRole("button", { name: "Edit mode" })
          .click();

        const toolbar = factoryGraphCardScope(browserPage.page).getByRole(
          "region",
          {
            name: "Factory graph editor tools",
          },
        );
        await toolbar.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        await toolbar.getByRole("button", { name: "Add" }).click();
        await browserPage.page
          .getByLabel("Add graph entity menu")
          .getByRole("button", { name: "Work type" })
          .evaluate((button) => button.click());

        const addDialog = browserPage.page.getByRole("dialog", {
          name: "Add work type",
        });
        await addDialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await addDialog.getByLabel("Identifier").fill("essay");
        await addDialog.getByLabel("First state").fill("queued");
        await addDialog.getByRole("button", { name: "Add entity" }).click();

        const discardChangesButton = toolbar.getByRole("button", {
          name: "Discard changes",
        });
        await expect
          .poll(async () => await discardChangesButton.isEnabled(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(true);

        await discardChangesButton.click();
        expect(
          await toolbar
            .getByRole("button", { name: "Discard changes" })
            .isDisabled(),
        ).toBe(true);

        await browserPage.page
          .getByRole("button", { name: "Leave editor" })
          .click();

        await browserPage.page
          .getByRole("button", { name: "Edit mode" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await expect
          .poll(
            async () =>
              await toolbar.getByRole("button", { name: "Add" }).count(),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe(0);
        await toolbar
          .getByRole("button", {
            name: "Show or hide",
          })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        expect(saveRequests).toHaveLength(0);
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
