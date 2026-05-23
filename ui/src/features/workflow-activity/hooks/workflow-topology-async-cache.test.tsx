import { act, renderHook, waitFor } from "@testing-library/react";

import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "./workflow-topology-async-cache";

type Deferred<T> = {
  promise: Promise<T>;
  reject: (reason?: unknown) => void;
  resolve: (value: T) => void;
};

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });

  return { promise, reject, resolve };
}

describe("workflow topology async cache", () => {
  it("deduplicates concurrent same-topology requests", async () => {
    const cache = createWorkflowTopologyAsyncCache<{ value: string }>();
    const deferred = createDeferred<{ value: string }>();
    const loadLayout = vi.fn(() => deferred.promise);
    const mapResolvedLayout = vi.fn((layout: { value: string }) => layout.value);

    const first = renderHook(() =>
      useWorkflowTopologyAsyncCache({
        cache,
        dependencies: [],
        fallbackValue: "fallback",
        initialValue: "initial",
        loadLayout,
        mapResolvedLayout,
        topologyKey: "workflow:a",
      }),
    );
    const second = renderHook(() =>
      useWorkflowTopologyAsyncCache({
        cache,
        dependencies: [],
        fallbackValue: "fallback",
        initialValue: "initial",
        loadLayout,
        mapResolvedLayout,
        topologyKey: "workflow:a",
      }),
    );

    expect(loadLayout).toHaveBeenCalledTimes(1);
    expect(first.result.current).toBe("initial");
    expect(second.result.current).toBe("initial");

    await act(async () => {
      deferred.resolve({ value: "resolved" });
      await deferred.promise;
    });

    await waitFor(() => {
      expect(first.result.current).toBe("resolved");
      expect(second.result.current).toBe("resolved");
    });
    expect(mapResolvedLayout).toHaveBeenCalledTimes(2);
  });

  it("reuses cached successful layouts for repeat requests", async () => {
    const cache = createWorkflowTopologyAsyncCache<{ value: string }>();
    const deferred = createDeferred<{ value: string }>();
    const loadLayout = vi.fn(() => deferred.promise);

    const first = renderHook(() =>
      useWorkflowTopologyAsyncCache({
        cache,
        dependencies: [],
        fallbackValue: "fallback",
        initialValue: "initial",
        loadLayout,
        mapResolvedLayout: (layout) => layout.value,
        topologyKey: "workflow:a",
      }),
    );

    await act(async () => {
      deferred.resolve({ value: "cached-result" });
      await deferred.promise;
    });

    await waitFor(() => {
      expect(first.result.current).toBe("cached-result");
    });

    const repeated = renderHook(() =>
      useWorkflowTopologyAsyncCache({
        cache,
        dependencies: [],
        fallbackValue: "fallback",
        initialValue: "initial",
        loadLayout,
        mapResolvedLayout: (layout) => layout.value,
        topologyKey: "workflow:a",
      }),
    );

    await waitFor(() => {
      expect(repeated.result.current).toBe("cached-result");
    });
    expect(loadLayout).toHaveBeenCalledTimes(1);
  });
});
