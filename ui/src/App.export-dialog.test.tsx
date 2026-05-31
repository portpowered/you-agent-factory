import "../testing/bun-app-shell-module-mocks";
import { useCurrentFactoryDocumentMock } from "../testing/bun-app-shell-module-mocks";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "bun:test";
import { useExportDialogStore } from "./features/export/state/exportDialogStore";
import * as factoryPngExportModule from "./features/export/lib/factory-png-export";
import type { FactoryValue } from "./api/named-factory";
import {
  baselineSnapshot,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";
import {
  currentNamedFactoryExportResponse,
  currentSessionFactoryExportAPIResponse,
  createDeferredPromise,
  exportTimelineEvents,
  exportImageFile,
  fromBase64,
  installExportDownloadProbe,
  createCurrentFactoryDocumentQueryResult,
  mockExportCurrentFactoryDocumentLoaded,
  toArrayBuffer,
} from "./testing/app-shell-export-test-utils";
import type { CurrentFactoryDocument } from "./api/current-factory-definition";

describe("App shell export dialog flows", () => {
  registerAppDashboardTestLifecycle();

  it("opens the export dialog from the toolbar and dismisses it without dashboard side effects", async () => {
    const exportProbe = installExportDownloadProbe();
    mockExportCurrentFactoryDocumentLoaded();
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });

    try {
      fireEvent.click(
        await screen.findByRole("button", { name: "Export PNG" }),
      );

      const dialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      await waitFor(() => {
        expect(
          within(dialog).getByDisplayValue("semantic-workflow"),
        ).toBeTruthy();
      });
      expect(
        within(dialog).getByText(/without changing the live dashboard state/i),
      ).toBeTruthy();
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
      expect(
        within(dialog).getByText("Selected image: cover.png"),
      ).toBeTruthy();

      fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

      await waitFor(() => {
        expect(
          screen.queryByRole("dialog", { name: "Export factory" }),
        ).toBeNull();
      });
      expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
      expect(exportProbe.getDownloadedBlob()).toBeNull();
      expect(exportProbe.getDownloadedFilename()).toBe("");
    } finally {
      exportProbe.restore();
    }
  });

  it("waits for a fresh current-factory response before exporting after reopen", async () => {
    const refreshedCurrentFactoryExportResponse = {
      ...currentNamedFactoryExportResponse,
      metadata: {
        ...currentNamedFactoryExportResponse.metadata,
        contractSource: "refetched-current-factory-api",
      },
      id: "authored-refetched-factory",
      name: "imported-workflow",
      version: {
        logical: "2",
        physical: "2026-04-16T12:05:00Z",
      },
    } satisfies FactoryValue;
    const refreshedCurrentFactoryAPIResponse = {
      ...refreshedCurrentFactoryExportResponse,
      version: currentSessionFactoryExportAPIResponse.version,
    };
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockResolvedValue({
        blob: new Blob(
          [toArrayBuffer(fromBase64("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg=="))],
          { type: "image/png" },
        ),
        metadata: {
          ...refreshedCurrentFactoryExportResponse,
          schemaVersion: "portos.agent-factory.png.v1",
        },
        ok: true,
      });
    const exportDocumentRef: {
      current: CurrentFactoryDocument | null;
    } = {
      current: currentSessionFactoryExportAPIResponse,
    };
    useCurrentFactoryDocumentMock.mockImplementation(() => {
      if (exportDocumentRef.current == null) {
        return createCurrentFactoryDocumentQueryResult({
          data: undefined,
          isFetching: true,
          isPending: true,
          isSuccess: false,
        });
      }

      return createCurrentFactoryDocumentQueryResult({
        data: exportDocumentRef.current,
        isFetching: false,
        isPending: false,
        isSuccess: true,
      });
    });

    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });

    try {
      fireEvent.click(
        await screen.findByRole("button", { name: "Export PNG" }),
      );

      const firstDialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      await waitFor(() => {
        expect(
          within(firstDialog).getByDisplayValue("semantic-workflow"),
        ).toBeTruthy();
      });
      fireEvent.click(
        within(firstDialog).getByRole("button", { name: "Cancel" }),
      );
      await waitFor(() => {
        expect(
          screen.queryByRole("dialog", { name: "Export factory" }),
        ).toBeNull();
      });

      exportDocumentRef.current = null;
      fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));

      const secondDialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      expect(
        within(secondDialog).getByText(
          "Loading the current authored factory definition.",
        ),
      ).toBeTruthy();
      expect(writeFactoryExportPngSpy).not.toHaveBeenCalled();

      exportDocumentRef.current = refreshedCurrentFactoryAPIResponse;
      await act(async () => {
        useExportDialogStore.setState({ isExportDialogOpen: false });
      });
      await act(async () => {
        useExportDialogStore.setState({ isExportDialogOpen: true });
      });

      const refreshedDialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      await waitFor(() => {
        expect(
          within(refreshedDialog).getByDisplayValue("imported-workflow"),
        ).toBeTruthy();
      });

      const imageInput = within(refreshedDialog).getByLabelText(
        "Cover image",
      ) as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(
        within(refreshedDialog).getByRole("button", { name: "Export PNG" }),
      );

      const { version: _refetchedVersion, ...refreshedExportFactory } =
        refreshedCurrentFactoryExportResponse;

      await waitFor(() => {
        expect(writeFactoryExportPngSpy).toHaveBeenCalledWith({
          factory: refreshedExportFactory,
          image: expect.any(File),
        });
      });
    } finally {
      writeFactoryExportPngSpy.mockRestore();
    }
  });

  it("does not download after cancelling an export that is still in flight", async () => {
    const exportProbe = installExportDownloadProbe();
    const pendingExport =
      createDeferredPromise<
        Awaited<ReturnType<typeof factoryPngExportModule.writeFactoryExportPng>>
      >();
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockReturnValue(pendingExport.promise);
    mockExportCurrentFactoryDocumentLoaded();
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });

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
      fireEvent.change(within(dialog).getByLabelText("Factory name"), {
        target: { value: "Factory Poster" },
      });
      fireEvent.change(imageInput);
      fireEvent.click(
        within(dialog).getByRole("button", { name: "Export PNG" }),
      );
      expect(
        (
          within(dialog).getByRole("button", {
            name: "Exporting...",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(true);
      fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

      await waitFor(() => {
        expect(
          screen.queryByRole("dialog", { name: "Export factory" }),
        ).toBeNull();
      });

      await act(async () => {
        pendingExport.resolve({
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
        await pendingExport.promise;
      });

      expect(writeFactoryExportPngSpy).toHaveBeenCalledTimes(1);
      expect(exportProbe.getDownloadedBlob()).toBeNull();
      expect(exportProbe.getDownloadedFilename()).toBe("");
    } finally {
      writeFactoryExportPngSpy.mockRestore();
      exportProbe.restore();
    }
  });
});
