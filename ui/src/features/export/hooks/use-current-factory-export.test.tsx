import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "bun:test";

import { getCurrentFactoryMock } from "../../../../testing/bun-named-factory-api-mocks";
import {
  NamedFactoryAPIError,
  type FactoryValue,
  getCurrentFactory,
} from "../../../api/named-factory";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { useCurrentFactoryExport } from "./use-current-factory-export";

const factory: FactoryValue = {
  id: "factory-aurora",
  name: "Factory Aurora",
  workers: [],
  workstations: [],
  workTypes: [],
};

describe("useCurrentFactoryExport", () => {
  beforeEach(() => {
    getCurrentFactoryMock.mockReset();
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
  });

  it("does not fetch while the export workflow is disabled", () => {
    const { result } = renderHook(() => useCurrentFactoryExport(false), {
      wrapper: createQueryClientWrapper(),
    });

    expect(getCurrentFactory).not.toHaveBeenCalled();
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

  it("reports a visible preparing state until the current factory loads", async () => {
    const pending = createDeferred<FactoryValue>();
    getCurrentFactoryMock.mockReturnValue(pending.promise);
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
      pending.resolve(factory);
      await pending.promise;
    });

    await waitFor(() => {
      expect(result.current).toEqual({
        currentFactoryExport: {
          factoryDefinition: factory,
          ok: true,
        },
        isPreparing: false,
      });
    });
  });

  it("loads export data from the selected non-default session route", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });
    getCurrentFactoryMock.mockResolvedValue(factory);

    renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper("session-beta"),
    });

    await waitFor(() => {
      expect(getCurrentFactory).toHaveBeenCalledWith({
        sessionID: "session-beta",
      });
    });
  });

  it("does not fetch export data when no session is selected", () => {
    const { result } = renderHook(() => useCurrentFactoryExport(true), {
      wrapper: createQueryClientWrapper(null),
    });

    expect(getCurrentFactory).not.toHaveBeenCalled();
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

  it("maps current-factory not-found errors to the unavailable export copy", async () => {
    getCurrentFactoryMock.mockRejectedValue(
      new NamedFactoryAPIError("Factory definition missing", { code: "NOT_FOUND" }),
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
    getCurrentFactoryMock.mockRejectedValue(new Error("Gateway timeout"));
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
