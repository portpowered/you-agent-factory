/**
 * Partial mocks for export dialog PNG/download seams.
 * Import before `export-factory-dialog` specs under Bun coverage batches.
 */
import { mock } from "bun:test";

const BROWSER_DOWNLOAD_MODULE =
  "../src/features/export/lib/browser-download";
const FACTORY_PNG_EXPORT_MODULE =
  "../src/features/export/lib/factory-png-export";

const browserDownloadActual = await import(BROWSER_DOWNLOAD_MODULE);
const factoryPngExportActual = await import(FACTORY_PNG_EXPORT_MODULE);

export const downloadBlobAsFileMock = mock(() => {
  throw new Error("downloadBlobAsFileMock not configured");
});
export const writeFactoryExportPngMock = mock(() => {
  throw new Error("writeFactoryExportPngMock not configured");
});

mock.module(BROWSER_DOWNLOAD_MODULE, () => ({
  ...browserDownloadActual,
  downloadBlobAsFile: downloadBlobAsFileMock,
}));

mock.module(FACTORY_PNG_EXPORT_MODULE, () => ({
  ...factoryPngExportActual,
  writeFactoryExportPng: writeFactoryExportPngMock,
}));
