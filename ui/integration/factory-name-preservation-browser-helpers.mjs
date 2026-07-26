import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { expect } from "vitest";

import {
  exportCoverImagePath,
  uiInteractionTimeoutMs,
  waitForCapturedDownloadOrDialogError,
  waitForDashboardSyncPreflight,
  waitForDashboardWidgetPicker,
  waitForDialogHidden,
  waitForDurableControlEnabled,
} from "./browser-test-harness.mjs";

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
  const syncPreflightResponse = waitForDashboardSyncPreflight(page);
  await page.goto(previewURL, {
    waitUntil: "domcontentloaded",
  });
  await syncPreflightResponse;
  await waitForDashboardWidgetPicker(page);
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
  const exportDialogButton = exportDialog.getByRole("button", {
    name: "Export PNG",
  });
  await waitForDurableControlEnabled(exportDialogButton);
  await exportDialogButton.click();

  await waitForCapturedDownloadOrDialogError(page, exportDialog);

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
  await waitForDialogHidden(exportDialog);

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
