import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { expect } from "vitest";

import {
  exportCoverImagePath,
  fillWorkstationPromptBody,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

export const canonicalSessionFactoryName = "Current Factory";

export const editableGraphFactoryDefinition = {
  metadata: {
    owner: "operations",
  },
  name: canonicalSessionFactoryName,
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

export const editableGraphFactoryReplayLines = [
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

export const exportFactoryDefinition = {
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

export function factoryGraphCardScope(page) {
  return page.getByRole("article", { name: "Factory graph" });
}

export async function installCapturedDownloadHook(page) {
  await page.addInitScript(() => {
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
}

export async function waitForDashboardReady(page, previewURL, server) {
  await page.goto(previewURL, {
    waitUntil: "domcontentloaded",
  });
  await page
    .getByRole("heading", { level: 1, name: "U", exact: true })
    .waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  await server.replayCompleted;
}

export async function exportFactoryPngFromDashboard(page, exportName) {
  await page.getByRole("button", { name: "Export PNG" }).waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await page.getByRole("button", { name: "Export PNG" }).click();

  const exportDialog = page.getByRole("dialog", {
    name: "Export factory",
  });
  await exportDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });

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
    page
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

  const download = await page.evaluate(
    () => window.__agentFactoryCapturedDownloads[0] ?? null,
  );
  expect(download).not.toBeNull();

  const downloadDirectory = await mkdtemp(
    path.join(os.tmpdir(), "agent-factory-name-preservation-"),
  );
  const downloadPath = path.join(downloadDirectory, download.filename);
  await writeFile(downloadPath, new Uint8Array(download.bytes));
  await exportDialog
    .getByRole("button", { exact: true, name: "Close" })
    .click();
  await page.getByRole("heading", { name: "Export factory" }).waitFor({
    state: "hidden",
    timeout: uiInteractionTimeoutMs,
  });

  return { download, downloadDirectory, downloadPath };
}

export async function openImportDialogForDroppedPng(
  page,
  downloadPath,
  download,
) {
  const exportedBytes = await readFile(downloadPath);
  const viewport = page.getByRole("region", {
    name: "Work graph viewport",
  });
  const importDataTransfer = await page.evaluateHandle(
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
  await page.getByText("Import factory PNG").waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await viewport.dispatchEvent("drop", { dataTransfer: importDataTransfer });

  const importDialog = page.getByRole("dialog", {
    name: "Review factory import",
  });
  await importDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });

  return importDialog;
}

export async function saveGraphEditorTopology(page) {
  const toolbar = factoryGraphCardScope(page).getByRole("region", {
    name: "Factory graph editor tools",
  });
  await toolbar.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });

  await toolbar.getByRole("button", { name: "Add" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Workstation" })
    .evaluate((button) => button.click());

  const addDialog = page.getByRole("dialog", {
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

  await saveChangesButton.focus();
  await saveChangesButton.press("Enter");
  const saveDialog = page.getByRole("dialog", {
    name: "Save factory graph changes?",
  });
  await saveDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await saveDialog.getByRole("button", { name: "Save topology" }).click();
}
