import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { DashboardTrace } from "../../../api/dashboard/types";
import {
  factoryTimelineEntryKey,
  useFactoryTimelineStore,
} from "../../timeline/public/store";
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";
import { dashboardTraceQueryKey, useDashboardTrace } from "./useTrace";

const streamIdentityA: StreamDerivedCacheIdentity = {
  backendScopeID: "backend-scope",
  factorySessionID: "session-a",
  logicalSessionKeyID: "logical-a",
  streamGenerationID: "generation-a",
};

const streamIdentityB: StreamDerivedCacheIdentity = {
  backendScopeID: "backend-scope",
  factorySessionID: "session-b",
  logicalSessionKeyID: "logical-b",
  streamGenerationID: "generation-b",
};

function buildTrace(traceID: string): DashboardTrace {
  return {
    dispatches: [],
    relations: [],
    request_ids: [],
    trace_id: traceID,
    transition_ids: [],
    work_ids: ["work-shared"],
    work_items: [],
    workstation_sequence: [],
  };
}

function seedTrace(
  streamIdentity: StreamDerivedCacheIdentity,
  trace: DashboardTrace,
): void {
  const entryKey = factoryTimelineEntryKey(streamIdentity);
  const current = useFactoryTimelineStore.getState();
  if (!current.entriesByKey[entryKey]) {
    current.activateEntry(streamIdentity);
  }

  useFactoryTimelineStore.setState((state) => {
    const entry = state.entriesByKey[entryKey];
    if (!entry) {
      throw new Error("Expected the trace test timeline entry to exist.");
    }

    const snapshot = entry.worldViewCache[0];
    if (!snapshot) {
      throw new Error("Expected the trace test timeline snapshot to exist.");
    }
    return {
      entriesByKey: {
        ...state.entriesByKey,
        [entryKey]: {
          ...entry,
          selectedTick: 0,
          worldViewCache: {
            0: {
              ...snapshot,
              tracesByWorkID: { "work-shared": trace },
            },
          },
        },
      },
    };
  });
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useDashboardTrace session and stream identity", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    useFactoryTimelineStore.getState().reset();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { gcTime: 0, retry: false },
      },
    });
  });

  afterEach(() => {
    useFactoryTimelineStore.getState().reset();
    queryClient.clear();
  });

  it("keeps a late session A read out of the selected session B view", async () => {
    const traceA = buildTrace("trace-session-a");
    const traceB = buildTrace("trace-session-b");
    seedTrace(streamIdentityA, traceA);
    seedTrace(streamIdentityB, traceB);

    const { result, rerender } = renderHook(
      ({ streamIdentity }: { streamIdentity: StreamDerivedCacheIdentity }) =>
        useDashboardTrace("work-shared", null, streamIdentity),
      {
        initialProps: { streamIdentity: streamIdentityA },
        wrapper: createWrapper(queryClient),
      },
    );

    await waitFor(() => {
      expect(result.current.data?.trace_id).toBe("trace-session-a");
    });

    act(() => {
      rerender({ streamIdentity: streamIdentityB });
    });

    await waitFor(() => {
      expect(result.current.data?.trace_id).toBe("trace-session-b");
    });

    const lateTraceA = buildTrace("trace-session-a-late");
    act(() => {
      seedTrace(streamIdentityA, lateTraceA);
      queryClient.setQueryData(
        dashboardTraceQueryKey("work-shared", null, 0, streamIdentityA),
        lateTraceA,
      );
    });

    expect(result.current.data?.trace_id).toBe("trace-session-b");
    expect(
      queryClient.getQueryData(
        dashboardTraceQueryKey("work-shared", null, 0, streamIdentityB),
      ),
    ).toMatchObject({ trace_id: "trace-session-b" });
  });
});
