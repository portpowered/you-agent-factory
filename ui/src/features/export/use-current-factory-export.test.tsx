import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { getCurrentFactory, NamedFactoryAPIError, type FactoryValue } from "../../api/named-factory";
import { useCurrentFactoryExport } from "./use-current-factory-export";

vi.mock("../../api/named-factory", async () => {
  const actual = await vi.importActual("../../api/named-factory");

  return {
    ...actual,
    getCurrentFactory: vi.fn(),
  };
});

const factory: FactoryValue = {
  id: "factory-aurora",
  name: "Factory Aurora",
  workers: [],
  workstations: [],
  workTypes: [],
};

describe("useCurrentFactoryExport", () => {
  beforeEach(() => {
    vi.mocked(getCurrentFactory).mockReset();
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
    vi.mocked(getCurrentFactory).mockReturnValue(pending.promise);
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

  it("maps current-factory not-found errors to the unavailable export copy", async () => {
    vi.mocked(getCurrentFactory).mockRejectedValue(
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
    vi.mocked(getCurrentFactory).mockRejectedValue(new Error("Gateway timeout"));
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

function createQueryClientWrapper(): ({ children }: { children: ReactNode }) => ReactNode {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
      },
    },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }): ReactNode {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
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
