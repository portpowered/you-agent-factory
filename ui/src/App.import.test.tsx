import {
  act,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { FactoryOrchestratorKind } from "./api/generated/openapi";
import * as factoryPngExportModule from "./features/export/lib/factory-png-export";
import * as factoryPngImportModule from "./features/import/lib/factory-png-import";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";
import {
  createDeferredPromise,
  currentNamedFactoryExportResponse,
  exportImageFile,
  exportTimelineEvents,
  fromBase64,
} from "./testing/app-shell-export-test-utils";
import {
  baselineSnapshot,
  chainRenderAppFetchMock,
  createFactoryImportValue,
  createFileDropTransfer,
  DEFAULT_FACTORY_SESSION_ID,
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

const incrementedSessionFactoryVersion = {
  logical: "10",
  physical: "2026-05-18T14:25:00.001Z",
} as const;

function buildFactorySessionGetResponse() {
  return {
    factoryDir: "/workspace/default",
    folderPath: "/workspace",
    id: DEFAULT_FACTORY_SESSION_ID,
    isDefault: true,
    project: "default",
    runtime: {
      lifecycle: {
        startedAt: "2026-06-26T00:00:00Z",
        updatedAt: "2026-06-26T00:00:00Z",
      },
      orchestratorKind: FactoryOrchestratorKind.PETRI,
      progress: {
        categories: {
          failed: 0,
          initial: 0,
          processing: 0,
          terminal: 0,
        },
        factoryState: "IDLE",
        inFlightCount: 0,
        totalTokens: 0,
      },
      status: "IDLE",
      streamIdentity: {
        backendScopeID: "/workspace::test-backend",
        factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
        logicalSessionKeyID: "/workspace::default::",
        streamGenerationID: "2026-06-26T00:00:00Z",
      },
      usage: { resources: [] },
    },
    target: {
      kind: "default",
    },
  };
}

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
    const currentSessionFactory = {
      name: "Session Current Name",
      workTypes: [],
      workers: [],
      workstations: [],
      version: defaultSessionFactoryVersion,
    };
    const activationDeferred = createDeferredPromise<Response>();
    vi.spyOn(factoryPngImportModule, "readFactoryImportPng").mockResolvedValue({
      ok: true,
      value: importValue,
    });
    const { fetchMock } = renderApp({
      snapshot: importedFactorySnapshot,
    });

    chainRenderAppFetchMock(fetchMock, async (path, method) => {
      if (path === "/factory-sessions/~default" && method === "GET") {
        return jsonResponse(buildFactorySessionGetResponse());
      }

      if (path === "/factory-sessions/~default/factory" && method === "GET") {
        return jsonResponse(currentSessionFactory);
      }

      if (path === "/factory-sessions/~default/factory" && method === "PUT") {
        return activationDeferred.promise;
      }

      return undefined;
    });

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
      jsonResponse({
        ...currentSessionFactory,
        ...importValue.factory,
        name: currentSessionFactory.name,
        version: incrementedSessionFactoryVersion,
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
          body: JSON.stringify({
            mode: "REPLACE_CURRENT",
            factory: {
              name: currentSessionFactory.name,
              workTypes: importValue.factory.workTypes,
              workers: importValue.factory.workers,
              workstations: importValue.factory.workstations,
              version: incrementedSessionFactoryVersion,
            },
          }),
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

    chainRenderAppFetchMock(fetchMock, async (path, method) => {
      if (path === "/factory-sessions/~default" && method === "GET") {
        return jsonResponse(buildFactorySessionGetResponse());
      }

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

      return undefined;
    });

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

  it("activates a harness-built exported PNG through PUT without re-proving export dialog copy", async () => {
    const { version: _version, ...importedFactory } =
      currentNamedFactoryExportResponse;
    const importValue = {
      ...createFactoryImportValue(),
      factory: importedFactory,
    };
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
    vi.spyOn(factoryPngImportModule, "readFactoryImportPng").mockResolvedValue({
      ok: true,
      value: importValue,
    });
    const { fetchMock } = renderApp({
      snapshot: importedFactorySnapshot,
      timelineEvents: exportTimelineEvents,
    });

    chainRenderAppFetchMock(fetchMock, async (path, method, _input, init) => {
      if (path === "/factory-sessions/~default" && method === "GET") {
        return jsonResponse(buildFactorySessionGetResponse());
      }

      if (path === "/factory-sessions/~default/factory" && method === "GET") {
        return jsonResponse({
          ...currentNamedFactoryExportResponse,
          version: defaultSessionFactoryVersion,
        });
      }

      if (path === "/factory-sessions/~default/factory" && method === "PUT") {
        const parsed = JSON.parse(String(init?.body)) as {
          factory?: typeof currentNamedFactoryExportResponse;
        };
        return jsonResponse(parsed.factory ?? parsed);
      }

      return undefined;
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });
    fireEvent.drop(
      viewport,
      createFileDropTransfer([
        new File([mockedExportResult.blob], "semantic-workflow.png", {
          type: "image/png",
        }),
      ]),
    );

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });
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
        mode: "REPLACE_CURRENT",
        factory: {
          ...currentNamedFactoryExportResponse,
          version: incrementedSessionFactoryVersion,
        },
      });
    });
    expectNoPostFactoriesActivation(fetchMock);
    await waitFor(() => {
      const sessionPreflightCount = fetchMock.mock.calls.filter(([url, init]) => {
        const path = resolveFetchPath(url);
        return (
          path.startsWith("/factory-sessions/~default/sync-preflight") &&
          resolveFetchMethod(url, init) === "GET"
        );
      }).length;
      expect(sessionPreflightCount).toBeGreaterThanOrEqual(2);
    });

    const refreshedStream = MockEventSource.instances.at(-1);
    if (!refreshedStream) {
      throw new Error("expected a dashboard stream after factory activation");
    }

    act(() => {
      refreshedStream.emit("snapshot", importedFactorySnapshot);
    });

    await waitFor(() => {
      const state = useFactoryTimelineStore.getState();
      expect(state.selectedTick).toBe(importedFactorySnapshot.tick_count);
      expect(state.worldViewCache[state.selectedTick]?.factory_state).toBe(
        importedFactorySnapshot.factory_state,
      );
    });
  });
});
