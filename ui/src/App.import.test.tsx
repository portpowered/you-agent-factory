import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import * as factoryPngExportModule from "./features/export/lib/factory-png-export";
import * as factoryPngImportModule from "./features/import/lib/factory-png-import";
import {
  MockEventSource,
  baselineSnapshot,
  createFactoryImportValue,
  createFileDropTransfer,
  importedFactorySnapshot,
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
} from "./testing/app-shell-export-test-utils";

describe("App shell import flows", () => {
  registerAppDashboardTestLifecycle();

  it("renders the operator graph for an empty runtime snapshot", async () => {
    renderApp({ snapshot: baselineSnapshot });

    expect(
      await screen.findByRole("heading", { name: "you-agent-factory" }),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Factory graph" })).toBeTruthy();
    expect(screen.getByText("In progress")).toBeTruthy();
    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    expect(
      within(screen.getByRole("region", { name: "Work graph viewport" })).getByRole("button", {
        name: "Zoom In",
      }),
    ).toBeTruthy();
    expect(
      screen.getByText("Waiting for more ticks"),
    ).toBeTruthy();
    expect(screen.queryByText("Idle")).toBeNull();
    expect(screen.queryByText("Live Workstation Dashboard")).toBeNull();
    expect(
      screen.queryByText(
        /Reconstruction-first workflow graph with live workstation overlays/i,
      ),
    ).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Terminal summary" }),
    ).toBeNull();
  });

  it("posts the dropped factory import as a direct canonical /factories activation payload", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    vi.spyOn(factoryPngImportModule, "readFactoryImportPng").mockResolvedValue({
      ok: true,
      value: importValue,
    });
    const { fetchMock } = renderApp({
      snapshot: importedFactorySnapshot,
    });

    fetchMock.mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path =
          typeof input === "string"
            ? input
            : input instanceof URL
              ? `${input.pathname}${input.search}`
              : input.url;

        if (path === "/factories") {
          return new Response(JSON.stringify(importValue.factory), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          });
        }

        throw new Error(
          `unexpected fetch for ${path} (${init?.method ?? "GET"})`,
        );
      },
    );

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });
    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Activate factory" }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factories",
        expect.objectContaining({
          body: JSON.stringify(importValue.factory),
          headers: {
            "Content-Type": "application/json",
          },
          method: "POST",
        }),
      );
    });
  });

  it("smoke tests authored export and dropped import as one dashboard-shell roundtrip", async () => {
    const exportProbe = installExportDownloadProbe();
    const mockedExportResult =
      await factoryPngExportModule.writeFactoryExportPng({
        factory: currentNamedFactoryExportResponse,
        image: exportImageFile(),
        rasterizeImageToPngBytes: async () => fromBase64(
          "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==",
        ),
      });
    if (!mockedExportResult.ok) {
      throw new Error(
        "expected the roundtrip export fixture to build successfully",
      );
    }
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockResolvedValue(mockedExportResult);
    const { fetchMock } = renderApp({
      snapshot: importedFactorySnapshot,
      timelineEvents: exportTimelineEvents,
    });

    fetchMock.mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path =
          typeof input === "string"
            ? input
            : input instanceof URL
              ? `${input.pathname}${input.search}`
              : input.url;

        if (path === "/factory-sessions/~default/factory") {
          return jsonResponse(currentNamedFactoryExportResponse);
        }

        if (path === "/factories") {
          return jsonResponse(JSON.parse(String(init?.body)));
        }

        throw new Error(
          `unexpected fetch for ${path} (${init?.method ?? "GET"})`,
        );
      },
    );

    try {
      fireEvent.click(
        await screen.findByRole("button", { name: "Export PNG" }),
      );

      const exportDialog = await screen.findByRole("dialog", {
        name: "Export factory",
      });
      await waitFor(() => {
        expect(
          (
            within(exportDialog).getByRole("button", {
              name: "Export PNG",
            }) as HTMLButtonElement
          ).disabled,
        ).toBe(false);
      });

      const imageInput = within(exportDialog).getByLabelText(
        "Cover image",
      ) as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(
        within(exportDialog).getByRole("button", { name: "Export PNG" }),
      );

      await waitFor(() => {
        expect(exportProbe.getDownloadedBlob()).not.toBeNull();
      });
      await waitFor(() => {
        expect(exportProbe.getDownloadedFilename()).toBe(
          "semantic-workflow.png",
        );
      });
      await waitFor(() => {
        expect(within(exportDialog).getByRole("status").textContent).toContain(
          "Downloaded semantic-workflow.png.",
        );
      });
      fireEvent.click(
        within(exportDialog).getByRole("button", { name: "Close" }),
      );
      await waitFor(() => {
        expect(
          screen.queryByRole("dialog", { name: "Export factory" }),
        ).toBeNull();
      });

      const exportedBlob = exportProbe.getDownloadedBlob();
      if (!(exportedBlob instanceof Blob)) {
        throw new Error("expected the export flow to download a PNG blob");
      }

      const viewport = await screen.findByRole("region", {
        name: "Work graph viewport",
      });
      fireEvent.drop(
        viewport,
        createFileDropTransfer([
          new File([exportedBlob], exportProbe.getDownloadedFilename(), {
            type: "image/png",
          }),
        ]),
      );

      const previewDialog = await screen.findByRole("dialog", {
        name: "Review factory import",
      });
      expect(previewDialog.textContent).toContain(
        currentNamedFactoryExportResponse.name,
      );
      expect(previewDialog.textContent).toContain("semantic-workflow.png");

      fireEvent.click(
        within(previewDialog).getByRole("button", { name: "Activate factory" }),
      );

      await waitFor(() => {
        const activationCall = fetchMock.mock.calls.find(
          ([url]) => url === "/factories",
        );
        expect(activationCall).toBeDefined();
        expect(activationCall?.[1]).toEqual(
          expect.objectContaining({
            body: expect.any(String),
            headers: {
              "Content-Type": "application/json",
            },
            method: "POST",
          }),
        );
        expect(JSON.parse(String(activationCall?.[1]?.body))).toEqual(
          currentNamedFactoryExportResponse,
        );
      });
      await waitFor(() => {
        expect(MockEventSource.instances.length).toBeGreaterThan(0);
      });

      const refreshedStream = MockEventSource.instances.at(-1);
      if (!refreshedStream) {
        throw new Error("expected a dashboard stream after factory activation");
      }

      act(() => {
        refreshedStream.emit("snapshot", importedFactorySnapshot);
      });

      await waitFor(() => {
        expect(screen.queryByText("Imported factory active")).toBeNull();
      });
      expect(
        await screen.findByRole("button", {
          name: "Select Review workstation",
        }),
      ).toBeTruthy();
    } finally {
      writeFactoryExportPngSpy.mockRestore();
      exportProbe.restore();
    }
  });
});
