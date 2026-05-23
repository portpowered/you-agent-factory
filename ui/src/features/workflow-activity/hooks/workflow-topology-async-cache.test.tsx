// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: shared async-cache lifecycle coverage is easier to follow in one describe block.
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

  it("falls back when a replacement topology load fails", async () => {
    const cache = createWorkflowTopologyAsyncCache<{ value: string }>();
    const firstDeferred = createDeferred<{ value: string }>();
    const secondDeferred = createDeferred<{ value: string }>();
    const loadLayout = vi.fn(
      ({ topologyKey }: { topologyKey: string }) =>
        topologyKey === "workflow:a"
          ? firstDeferred.promise
          : secondDeferred.promise,
    );

    const { result, rerender } = renderHook(
      ({ topologyKey }) =>
        useWorkflowTopologyAsyncCache({
          cache,
          dependencies: [topologyKey],
          fallbackValue: "fallback",
          initialValue: "initial",
          loadLayout: () => loadLayout({ topologyKey }),
          mapResolvedLayout: (layout) => layout.value,
          topologyKey,
        }),
      {
        initialProps: {
          topologyKey: "workflow:a",
        },
      },
    );

    await act(async () => {
      firstDeferred.resolve({ value: "resolved-a" });
      await firstDeferred.promise;
    });

    await waitFor(() => {
      expect(result.current).toBe("resolved-a");
    });

    rerender({ topologyKey: "workflow:b" });

    await act(async () => {
      secondDeferred.reject(new Error("layout failed"));
      await secondDeferred.promise.catch(() => undefined);
    });

    await waitFor(() => {
      expect(result.current).toBe("fallback");
    });
  });

  it("ignores stale async completions after the topology key changes", async () => {
    const cache = createWorkflowTopologyAsyncCache<{ value: string }>();
    const firstDeferred = createDeferred<{ value: string }>();
    const secondDeferred = createDeferred<{ value: string }>();

    const { result, rerender } = renderHook(
      ({ topologyKey }) =>
        useWorkflowTopologyAsyncCache({
          cache,
          dependencies: [topologyKey],
          fallbackValue: "fallback",
          initialValue: "initial",
          loadLayout: () =>
            topologyKey === "workflow:a"
              ? firstDeferred.promise
              : secondDeferred.promise,
          mapResolvedLayout: (layout) => layout.value,
          topologyKey,
        }),
      {
        initialProps: {
          topologyKey: "workflow:a",
        },
      },
    );

    rerender({ topologyKey: "workflow:b" });

    await act(async () => {
      firstDeferred.resolve({ value: "stale-a" });
      await firstDeferred.promise;
    });

    expect(result.current).toBe("initial");

    await act(async () => {
      secondDeferred.resolve({ value: "resolved-b" });
      await secondDeferred.promise;
    });

    await waitFor(() => {
      expect(result.current).toBe("resolved-b");
    });
  });
});
