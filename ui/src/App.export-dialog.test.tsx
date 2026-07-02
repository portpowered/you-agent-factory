import "./testing/app-shell-work-outcome-stub";
import "./testing/app-shell-workflow-activity-stub";

import {
  act,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ImportFactoryValue } from "./api/session-factory";
import { DEFAULT_FACTORY_SESSION_ID } from "./api/session-routing";
import * as factoryPngExportModule from "./features/export/lib/factory-png-export";
import {
  createDeferredPromise,
  currentNamedFactoryExportResponse,
  currentSessionFactoryExportAPIResponse,
  exportImageFile,
  exportTimelineEvents,
  fromBase64,
  installExportDownloadProbe,
  toArrayBuffer,
} from "./testing/app-shell-export-test-utils";
import {
  baselineSnapshot,
  chainRenderAppFetchMock,
  fetchCallPaths,
  jsonResponse,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
  renderApp,
  waitForDashboardShell,
} from "./testing/app-shell-test-utils";

describe("App shell export dialog flows", () => {
  registerAppDashboardTestLifecycle();

  it("opens the export dialog from the toolbar and dismisses it without dashboard side effects", async () => {
    const exportProbe = installExportDownloadProbe();
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    chainRenderAppFetchMock(fetchMock, async (path, method) => {
      if (
        method === "GET" &&
        path === "/factory-sessions/~default/factory"
      ) {
        return jsonResponse(currentSessionFactoryExportAPIResponse);
      }

      return undefined;
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
      expect(
        fetchCallPaths(fetchMock).filter((path) => path.endsWith("/factory")),
      ).toContain(`/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/factory`);
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
    } satisfies ImportFactoryValue;
    const refreshedCurrentFactoryAPIResponse = {
      ...refreshedCurrentFactoryExportResponse,
      version: currentSessionFactoryExportAPIResponse.version,
    };
    const refreshedCurrentFactoryResponse = createDeferredPromise<Response>();
    let serveDeferredFactoryResponse = false;
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
          ...refreshedCurrentFactoryExportResponse,
          schemaVersion: "portos.agent-factory.png.v1",
        },
        ok: true,
      });
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
      fetchOverride: async (path, method) => {
        if (
          method !== "GET" ||
          path !== `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/factory`
        ) {
          return undefined;
        }

        if (serveDeferredFactoryResponse) {
          serveDeferredFactoryResponse = false;
          return refreshedCurrentFactoryResponse.promise;
        }

        return jsonResponse(currentSessionFactoryExportAPIResponse);
      },
    });

    try {
      await waitForDashboardShell();
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

      serveDeferredFactoryResponse = true;
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

      await act(async () => {
        refreshedCurrentFactoryResponse.resolve(
          jsonResponse(refreshedCurrentFactoryAPIResponse),
        );
        await refreshedCurrentFactoryResponse.promise;
      });

      await waitFor(() => {
        expect(
          within(secondDialog).getByDisplayValue("imported-workflow"),
        ).toBeTruthy();
      });

      const imageInput = within(secondDialog).getByLabelText(
        "Cover image",
      ) as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(
        within(secondDialog).getByRole("button", { name: "Export PNG" }),
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
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    chainRenderAppFetchMock(fetchMock, async (path, method) => {
      if (
        method === "GET" &&
        path === "/factory-sessions/~default/factory"
      ) {
        return jsonResponse(currentSessionFactoryExportAPIResponse);
      }

      return undefined;
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
