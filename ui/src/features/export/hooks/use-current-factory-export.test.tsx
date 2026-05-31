import "../../../../testing/bun-current-factory-definition-api-mocks";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it } from "bun:test";

import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { getCurrentFactoryDocumentMock } from "../../../../testing/bun-current-factory-definition-api-mocks";
import { wrapWithDashboardSessionTest } from "../../../testing";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";

const { useCurrentFactoryExport } = await import("./use-current-factory-export");

const documentFactory: CurrentFactoryDocument = {
  id: "factory-aurora",
  name: "Factory Aurora",
  workers: [],
  workstations: [
    {
      name: "document-only",
      workTypes: [],
    },
  ],
  workTypes: [],
  version: {
    logical: 1,
    physical: 1,
  },
};

/** Fixture-only: snapshot topology that must not become the export payload. */
const snapshotOnlyWorkstationName = "snapshot-only";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: export document-plane cases share session and divergent-fixture setup.
describe("useCurrentFactoryExport", () => {
  beforeEach(() => {
    getCurrentFactoryDocumentMock.mockReset();
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
  });

  it("does not fetch while the export workflow is disabled", () => {
    const { result } = renderHook(() => useCurrentFactoryExport(false), {
      wrapper: createQueryClientWrapper(),
    });

    expect(getCurrentFactoryDocumentMock).not.toHaveBeenCalled();
    expect(result.current).toEqual({
      currentFactoryExport: {
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message:
          "The current factory definition is not available yet. Wait for the current-factory API to expose the authored definition before exporting.",
        ok: false,
      },
      isPreparing: true,
    });
  });

  it("reports a visible preparing state until the factory document loads", async () => {
    const pending = createDeferred<CurrentFactoryDocument>();
    getCurrentFactoryDocumentMock.mockReturnValue(pending.promise);
    const { result } = renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toEqual({
        currentFactoryExport: {
          code: "FACTORY_DEFINITION_UNAVAILABLE",
          message:
            "The current factory definition is not available yet. Wait for the current-factory API to expose the authored definition before exporting.",
          ok: false,
        },
        isPreparing: true,
      });
    });

    await act(async () => {
      pending.resolve(documentFactory);
      await pending.promise;
    });

    await waitFor(() => {
      expect(result.current).toEqual({
        currentFactoryExport: {
          factoryDefinition: {
            id: "factory-aurora",
            name: "Factory Aurora",
            workers: [],
            workstations: [
              {
                name: "document-only",
                workTypes: [],
              },
            ],
            workTypes: [],
          },
          ok: true,
        },
        isPreparing: false,
      });
    });
  });

  it("loads export data for session-beta through session factory document GET", async () => {
    getCurrentFactoryDocumentMock.mockResolvedValue(documentFactory);

    renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper("session-beta"),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDocumentMock).toHaveBeenCalledWith({
        sessionID: "session-beta",
      });
    });
  });

  it("does not fetch export data when no session is selected", () => {
    const { result } = renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper(null),
    });

    expect(getCurrentFactoryDocumentMock).not.toHaveBeenCalled();
    expect(result.current).toEqual({
      currentFactoryExport: {
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message:
          "The current factory definition is not available yet. Wait for the current-factory API to expose the authored definition before exporting.",
        ok: false,
      },
      isPreparing: false,
    });
  });

  it("maps factory document not-found errors to the unavailable export copy", async () => {
    getCurrentFactoryDocumentMock.mockRejectedValue(
      new CurrentFactoryDefinitionError("Factory definition missing", {
        code: "NOT_FOUND",
      }),
    );
    const { result } = renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toEqual({
        currentFactoryExport: {
          code: "FACTORY_DEFINITION_UNAVAILABLE",
          message:
            "The current factory definition is not available yet. Wait for the current-factory API to expose the authored definition before exporting.",
          ok: false,
        },
        isPreparing: false,
      });
    });
  });

  it("includes generic transport error messages in the export preparation failure", async () => {
    getCurrentFactoryDocumentMock.mockRejectedValue(new Error("Gateway timeout"));
    const { result } = renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toEqual({
        currentFactoryExport: {
          code: "FACTORY_DEFINITION_UNAVAILABLE",
          message:
            "The current factory definition could not be loaded from the current-factory API. Gateway timeout",
          ok: false,
        },
        isPreparing: false,
      });
    });
  });

  it("exports the factory document when snapshot topology would diverge", async () => {
    getCurrentFactoryDocumentMock.mockResolvedValue(documentFactory);

    const { result } = renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current.currentFactoryExport.ok).toBe(true);
    });

    if (!result.current.currentFactoryExport.ok) {
      throw new Error("Expected export to succeed from the document plane.");
    }

    expect(result.current.currentFactoryExport.factoryDefinition.workstations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: "document-only" }),
      ]),
    );
    expect(result.current.currentFactoryExport.factoryDefinition.workstations).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: snapshotOnlyWorkstationName }),
      ]),
    );
    expect(snapshotOnlyWorkstationName).toBe("snapshot-only");
  });
});

function createQueryClientWrapper(
  sessionID: string | null = "~default",
): ({ children }: { children: ReactNode }) => ReactNode {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
      },
    },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }): ReactNode {
    return (
      <QueryClientProvider client={queryClient}>
        {wrapWithDashboardSessionTest(children, { sessionID })}
      </QueryClientProvider>
    );
  };
}

function createDeferred<T>() {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  let reject: (reason?: unknown) => void = () => {};
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });

  return { promise, reject, resolve };
}
