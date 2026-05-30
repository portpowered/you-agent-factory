import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import * as factoryPngExportModule from "./features/export/lib/factory-png-export";
import { readFactoryImportPng } from "./features/import/lib/factory-png-import";
import type { FactoryValue } from "./api/named-factory";
import {
  baselineSnapshot,
  jsonResponse,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";
import {
  currentNamedFactoryExportResponse,
  exportImageFile,
  exportTimelineEvents,
  fromBase64,
  installExportDownloadProbe,
  toArrayBuffer,
} from "./testing/app-shell-export-test-utils";

const currentFactoryWithBundledFiles = {
  ...currentNamedFactoryExportResponse,
  supportingFiles: {
    bundledFiles: [
      {
        content: {
          encoding: "utf-8",
          inline: "#!/usr/bin/env bash\nprintf 'setup\\n'\n",
        },
        targetPath: "factory/scripts/setup-workspace.sh",
        type: "SCRIPT",
      },
      {
        content: {
          encoding: "utf-8",
          inline: "starter task\n",
        },
        targetPath: "factory/inputs/task/default/starter.md",
        type: "INPUT",
      },
    ],
  },
} satisfies FactoryValue;

describe("App shell export submission flows", () => {
  registerAppDashboardTestLifecycle();

  it("validates the export fields and accepts the confirmed name plus selected image", async () => {
    const exportProbe = installExportDownloadProbe();
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    fetchMock.mockResolvedValueOnce(
      jsonResponse(currentNamedFactoryExportResponse),
    );

    try {
      fireEvent.click(
        await screen.findByRole("button", { name: "Export PNG" }),
      );

      const dialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      await waitFor(() => {
        expect(
          (
            within(dialog).getByRole("button", {
              name: "Export PNG",
            }) as HTMLButtonElement
          ).disabled,
        ).toBe(false);
      });

      const exportButton = within(dialog).getByRole("button", {
        name: "Export PNG",
      });
      fireEvent.click(exportButton);
      expect(
        within(dialog).getByText("Choose a cover image before exporting."),
      ).toBeTruthy();

      const nameInput = within(dialog).getByLabelText("Factory name");
      fireEvent.change(nameInput, { target: { value: "   " } });
      fireEvent.click(exportButton);
      expect(
        within(dialog).getByText("Enter a factory name before exporting."),
      ).toBeTruthy();

      fireEvent.change(nameInput, { target: { value: "Factory Poster" } });
      const imageInput = within(dialog).getByLabelText(
        "Cover image",
      ) as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);

      expect(within(dialog).getByDisplayValue("Factory Poster")).toBeTruthy();
      expect(
        within(dialog).getByText("Selected image: cover.png"),
      ).toBeTruthy();
      expect(
        within(dialog).queryByText("Enter a factory name before exporting."),
      ).toBeNull();
      expect(
        within(dialog).queryByText("Choose a cover image before exporting."),
      ).toBeNull();
      expect(exportProbe.getDownloadedBlob()).toBeNull();
      expect(exportProbe.getDownloadedFilename()).toBe("");
    } finally {
      exportProbe.restore();
    }
  });

  it("exports the current named-factory API payload instead of the event timeline projection", async () => {
    const exportProbe = installExportDownloadProbe();
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockResolvedValue({
        blob: new Blob(
          [
            toArrayBuffer(
              fromBase64(
                "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==",
              ),
            ),
          ],
          { type: "image/png" },
        ),
        metadata: {
          ...currentNamedFactoryExportResponse,
          name: "Factory Poster",
          schemaVersion: "portos.agent-factory.png.v1",
        },
        ok: true,
      });
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    fetchMock.mockResolvedValueOnce(
      jsonResponse(currentNamedFactoryExportResponse),
    );

    try {
      fireEvent.click(
        await screen.findByRole("button", { name: "Export PNG" }),
      );

      const dialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      await waitFor(() => {
        expect(
          (
            within(dialog).getByRole("button", {
              name: "Export PNG",
            }) as HTMLButtonElement
          ).disabled,
        ).toBe(false);
      });
      fireEvent.change(within(dialog).getByLabelText("Factory name"), {
        target: { value: "Factory Poster" },
      });
      const imageInput = within(dialog).getByLabelText(
        "Cover image",
      ) as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(
        within(dialog).getByRole("button", { name: "Export PNG" }),
      );

      const { version: _exportVersion, ...exportFactoryWithoutVersion } =
        currentNamedFactoryExportResponse;

      await waitFor(() => {
        expect(writeFactoryExportPngSpy).toHaveBeenCalledWith({
          factory: {
            ...exportFactoryWithoutVersion,
            name: "Factory Poster",
          },
          image: expect.any(File),
        });
      });
      expect(exportProbe.getDownloadedFilename()).toBe("factory-poster.png");
    } finally {
      writeFactoryExportPngSpy.mockRestore();
      exportProbe.restore();
    }
  });

  it("embeds current-factory bundled files in readable PNG metadata", async () => {
    const exportProbe = installExportDownloadProbe();
    const writeFactoryExportPng = factoryPngExportModule.writeFactoryExportPng;
    let exportResultPromise: ReturnType<typeof writeFactoryExportPng> | null =
      null;
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockImplementation((options) => {
        exportResultPromise = writeFactoryExportPng({
          ...options,
          rasterizeImageToPngBytes: async () =>
            fromBase64(
              "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==",
            ),
        });
        return exportResultPromise;
      });
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    fetchMock.mockResolvedValueOnce(
      jsonResponse(currentFactoryWithBundledFiles),
    );

    try {
      fireEvent.click(
        await screen.findByRole("button", { name: "Export PNG" }),
      );

      const dialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      await waitFor(() => {
        expect(
          (
            within(dialog).getByRole("button", {
              name: "Export PNG",
            }) as HTMLButtonElement
          ).disabled,
        ).toBe(false);
      });

      const imageInput = within(dialog).getByLabelText(
        "Cover image",
      ) as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(
        within(dialog).getByRole("button", { name: "Export PNG" }),
      );

      const { version: _bundledVersion, ...bundledFactoryWithoutVersion } =
        currentFactoryWithBundledFiles;

      await waitFor(() => {
        expect(writeFactoryExportPngSpy).toHaveBeenCalledWith({
          factory: bundledFactoryWithoutVersion,
          image: expect.any(File),
        });
      });
      if (!exportResultPromise) {
        throw new Error("expected export flow to start writing PNG metadata");
      }
      const exportResult = await exportResultPromise;
      if (!exportResult.ok) {
        throw new Error(exportResult.error.message, {
          cause: exportResult.error.cause,
        });
      }
      await waitFor(() => {
        expect(exportProbe.getDownloadedFilename()).toBe(
          "semantic-workflow.png",
        );
      });

      const downloadedBlob = exportProbe.getDownloadedBlob();
      if (!downloadedBlob) {
        throw new Error("expected export flow to download a PNG blob");
      }

      const importResult = await readFactoryImportPng({
        createPreviewImageSrc: () => "blob:preview",
        file: downloadedBlob,
        revokePreviewImageSrc: () => {},
        validatePreviewImage: async () => {},
      });

      expect(importResult.ok).toBe(true);
      if (!importResult.ok) {
        throw new Error(importResult.error.message);
      }
      expect(importResult.value.factory.supportingFiles).toEqual(
        currentFactoryWithBundledFiles.supportingFiles,
      );
    } finally {
      writeFactoryExportPngSpy.mockRestore();
      exportProbe.restore();
    }
  });
});
