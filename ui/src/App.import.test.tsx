import "../testing/bun-app-shell-module-mocks";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "bun:test";
import * as factoryPngExportModule from "./features/export/lib/factory-png-export";
import * as factoryPngImportModule from "./features/import/lib/factory-png-import";
import {
  createDeferredPromise,
  currentNamedFactoryExportResponse,
  currentSessionFactoryExportAPIResponse,
  exportImageFile,
  exportTimelineEvents,
  fromBase64,
  installExportDownloadProbe,
  mockExportCurrentFactoryDocumentLoaded,
} from "./testing/app-shell-export-test-utils";
import {
  baselineSnapshot,
  createFactoryImportValue,
  createFileDropTransfer,
  importedFactorySnapshot,
  jsonResponse,
  MockEventSource,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";

const defaultSessionFactoryVersion = {
  logical: "9",
  physical: "2026-05-18T14:25:00Z",
} as const;
import {
  buildSessionFactoryActivationPutBody,
  incrementedSessionFactoryVersion,
  isSessionFactoryRequest,
  mergeImportedFactoryIntoSessionDocument,
  mockGetSessionFactory,
  mockPutSessionFactory,
  parseSessionFactoryPutFactory,
  sessionFactoryImportActivationDocument,
  sessionFactoryNamedExportDocument,
} from "./testing/session-factory-mocks";

function expectNoRetiredDashboardBranding(): void {
  expect(screen.queryByText(/finite you/i)).toBeNull();
  expect(screen.queryByText(/Infinite You/i)).toBeNull();
}

function resolveFetchPath(input: RequestInfo | URL): string {
  return typeof input === "string"
    ? input
    : input instanceof URL
      ? `${input.pathname}${input.search}`
      : input.url;
}

function resolveFetchMethod(
  input: RequestInfo | URL,
  init?: RequestInit,
): string {
  if (init?.method) {
    return init.method;
  }
  if (input instanceof Request) {
    return input.method;
  }
  return "GET";
}

function expectNoPostFactoriesActivation(
  fetchMock: ReturnType<typeof vi.fn>,
): void {
  const postFactoriesCall = fetchMock.mock.calls.find(([url, init]) => {
    const path = resolveFetchPath(url);
    return path === "/factories" && resolveFetchMethod(url, init) === "POST";
  });
  expect(postFactoriesCall).toBeUndefined();
}

describe("App shell import flows", () => {
  registerAppDashboardTestLifecycle();

  it("renders the operator graph for an empty runtime snapshot", async () => {
    renderApp({ snapshot: baselineSnapshot });

    expect(await screen.findByRole("heading", { name: "U" })).toBeTruthy();
    expectNoRetiredDashboardBranding();
    expect(screen.getByRole("heading", { name: "Factory graph" })).toBeTruthy();
    expect(screen.getByText("In progress")).toBeTruthy();
    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    expect(
      within(
        screen.getByRole("region", { name: "Work graph viewport" }),
      ).getByRole("button", {
        name: "Zoom In",
      }),
    ).toBeTruthy();
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
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

  it("activates the dropped factory import through PUT /factory-sessions/~default/factory", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activationDeferred = createDeferredPromise<Response>();
    vi.spyOn(factoryPngImportModule, "readFactoryImportPng").mockResolvedValue({
      ok: true,
      value: importValue,
    });
    const { fetchMock } = renderApp({
      snapshot: importedFactorySnapshot,
    });

    fetchMock.mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = resolveFetchPath(input);
        const method = resolveFetchMethod(input, init);

        if (isSessionFactoryRequest(path, method)) {
          if (method === "GET") {
            return mockGetSessionFactory({
              document: sessionFactoryImportActivationDocument,
            });
          }

          if (method === "PUT") {
            return activationDeferred.promise;
          }
        }

        throw new Error(`unexpected fetch for ${path} (${method})`);
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
      within(previewDialog).getByRole("button", { name: "Confirm import" }),
    );

    await waitFor(() => {
      expect(
        within(previewDialog).getByRole("button", {
          name: "Activating factory...",
        }),
      ).toBeTruthy();
    });
    expect(
      within(previewDialog).getByRole("button", { name: "Cancel import" }),
    ).toHaveProperty("disabled", true);

    activationDeferred.resolve(
      mockPutSessionFactory({
        responseDocument: mergeImportedFactoryIntoSessionDocument(
          sessionFactoryImportActivationDocument,
          importValue.factory,
        ),
      }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/~default/factory",
        {
          method: "GET",
        },
      );
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/~default/factory",
        expect.objectContaining({
          body: JSON.stringify(
            buildSessionFactoryActivationPutBody({
              sessionName: sessionFactoryImportActivationDocument.name,
              importedFactory: importValue.factory,
            }),
          ),
          headers: {
            "content-type": "application/json",
          },
          method: "PUT",
        }),
      );
    });
    expectNoPostFactoriesActivation(fetchMock);
  });

  it("activates create-new-named imports through UPSERT_NAMED_AND_ACTIVATE on the selected session", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const currentSessionFactory = {
      name: "Session Current Name",
      workTypes: [],
      workers: [],
      workstations: [],
      version: defaultSessionFactoryVersion,
    };
    vi.spyOn(factoryPngImportModule, "readFactoryImportPng").mockResolvedValue({
      ok: true,
      value: importValue,
    });
    const { fetchMock } = renderApp({
      snapshot: importedFactorySnapshot,
    });

    fetchMock.mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = resolveFetchPath(input);
        const method = resolveFetchMethod(input, init);

        if (path === "/factory-sessions/~default/factory" && method === "GET") {
          return jsonResponse(currentSessionFactory);
        }

        if (path === "/factory-sessions/~default/factory" && method === "PUT") {
          return jsonResponse({
            name: "Dropped Factory",
            ...importValue.factory,
            version: {
              logical: "1",
              physical: "2026-05-18T14:41:00Z",
            },
          });
        }

        throw new Error(`unexpected fetch for ${path} (${method})`);
      },
    );

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });
    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });
    const messages = within(previewDialog).getByRole("radiogroup", {
      name: "Import save choice",
    });
    fireEvent.click(
      within(messages).getByRole("radio", {
        name: /Create new named factory/i,
      }),
    );

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Confirm import" }),
    );

    await waitFor(() => {
      const putActivationCall = fetchMock.mock.calls.find(([url, init]) => {
        return (
          resolveFetchPath(url) === "/factory-sessions/~default/factory" &&
          resolveFetchMethod(url, init) === "PUT"
        );
      });
      expect(putActivationCall).toBeDefined();
      expect(JSON.parse(String(putActivationCall?.[1]?.body))).toEqual({
        mode: "UPSERT_NAMED_AND_ACTIVATE",
        factory: {
          name: "Dropped Factory-2",
          workTypes: importValue.factory.workTypes,
          workers: importValue.factory.workers,
          workstations: importValue.factory.workstations,
        },
      });
    });
    expectNoPostFactoriesActivation(fetchMock);
  });

  it("smoke tests authored export and dropped import as one dashboard-shell roundtrip", async () => {
    const exportProbe = installExportDownloadProbe();
    const mockedExportResult =
      await factoryPngExportModule.writeFactoryExportPng({
        factory: currentNamedFactoryExportResponse,
        image: exportImageFile(),
        rasterizeImageToPngBytes: async () =>
          fromBase64(
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
        const path = resolveFetchPath(input);
        const method = resolveFetchMethod(input, init);

        if (isSessionFactoryRequest(path, method)) {
          if (method === "GET") {
            return mockGetSessionFactory({
              document: {
                ...sessionFactoryNamedExportDocument,
                ...currentNamedFactoryExportResponse,
              },
            });
          }

          if (method === "PUT") {
            return mockPutSessionFactory({
              responseDocument: parseSessionFactoryPutFactory(
                String(init?.body),
              ),
            });
          }
        }

        throw new Error(`unexpected fetch for ${path} (${method})`);
      },
    );

    try {
      mockExportCurrentFactoryDocumentLoaded({
        ...sessionFactoryNamedExportDocument,
        ...currentSessionFactoryExportAPIResponse,
      });
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
        within(previewDialog).getByRole("button", { name: "Confirm import" }),
      );

      await waitFor(() => {
        const putActivationCall = fetchMock.mock.calls.find(([url, init]) => {
          return (
            resolveFetchPath(url) === "/factory-sessions/~default/factory" &&
            resolveFetchMethod(url, init) === "PUT"
          );
        });
        expect(putActivationCall).toBeDefined();
        expect(putActivationCall?.[1]).toEqual(
          expect.objectContaining({
            body: expect.any(String),
            headers: {
              "content-type": "application/json",
            },
            method: "PUT",
          }),
        );
        expect(JSON.parse(String(putActivationCall?.[1]?.body))).toEqual({
          mode: "REPLACE_CURRENT",
          factory: {
            ...currentNamedFactoryExportResponse,
            version: incrementedSessionFactoryVersion,
          },
        });
      });
      expectNoPostFactoriesActivation(fetchMock);
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
