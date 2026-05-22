import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import * as factoryPngExportModule from "./features/export/factory-png-export";
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
          [toArrayBuffer(fromBase64("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg=="))],
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

      await waitFor(() => {
        expect(writeFactoryExportPngSpy).toHaveBeenCalledWith({
          factory: {
            ...currentNamedFactoryExportResponse,
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
});
