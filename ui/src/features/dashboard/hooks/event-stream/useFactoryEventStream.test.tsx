import { act, renderHook, waitFor } from "@testing-library/react";

import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY,
  CURRENT_FACTORY_DOCUMENT_QUERY_KEY,
} from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";
import { useFactoryEventStream } from "./useFactoryEventStream";
import {
  CANONICAL_SELECTED_TICK_EVENTS,
  createFactoryEventStreamQueryClient,
} from "./useFactoryEventStream.fixtures";
import {
  createFactoryEventStreamTestWrapper,
  replayHarness,
  resetFactoryEventStreamStores,
  seedFactoryEventStreamStores,
} from "./useFactoryEventStream.test-support";

const RESOLVED_DEFAULT_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function resolvedDefaultStreamIdentity() {
  return {
    backendScopeID: "backend-scope-a",
    factorySessionID: RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyID: "logical-default",
    streamGenerationID: "2026-06-26T00:00:00Z",
  };
}

describe("useFactoryEventStream resolved session identity", () => {
  let queryClient = createFactoryEventStreamQueryClient();

  beforeEach(() => {
    replayHarness.install();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 200,
        }),
      ),
    );
    queryClient = createFactoryEventStreamQueryClient();
    seedFactoryEventStreamStores();
  });

  afterEach(() => {
    resetFactoryEventStreamStores();
    vi.unstubAllGlobals();
  });

  it("opens the resolved UUID session event stream for default-session selectors", async () => {
    const streamIdentity = resolvedDefaultStreamIdentity();

    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          onEvent: () => {},
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          streamIdentity,
        }),
      { wrapper: createFactoryEventStreamTestWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );
    expect(replayHarness.getStreams()[0]?.url).not.toBe("/events");
    expect(replayHarness.getStreams()[0]?.url).not.toContain("~default");
  });

  it("never opens the process-global /events stream for dashboard traffic", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          onEvent: () => {},
          sessionID: "session-beta",
          streamIdentity: {
            backendScopeID: "backend-scope-b",
            factorySessionID: "session-beta",
            logicalSessionKeyID: "logical-beta",
            streamGenerationID: "2026-06-26T00:00:00Z",
          },
        }),
      { wrapper: createFactoryEventStreamTestWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    for (const stream of replayHarness.getStreams()) {
      expect(stream.url).toMatch(/^\/factory-sessions\//);
      expect(stream.url).not.toBe("/events");
    }
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: transport lifecycle cases share one query client and event-stream harness.
describe("useFactoryEventStream transport", () => {
  let queryClient = createFactoryEventStreamQueryClient();
  const receivedEvents: unknown[] = [];

  beforeEach(() => {
    replayHarness.install();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 200,
        }),
      ),
    );
    receivedEvents.length = 0;
    queryClient = createFactoryEventStreamQueryClient();
    seedFactoryEventStreamStores();
  });

  afterEach(() => {
    resetFactoryEventStreamStores();
    vi.unstubAllGlobals();
  });

  it("opens the session-scoped events stream and delivers compacted events to onEvent", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          onEvent: (event) => {
            receivedEvents.push(event);
          },
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { wrapper: createFactoryEventStreamTestWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(1);
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await act(async () => {
      stream.emit("message", CANONICAL_SELECTED_TICK_EVENTS[0]);
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    await waitFor(() => {
      expect(receivedEvents).toHaveLength(1);
    });
  });

  it("does not open a stream when disabled for a paused session and keeps the paused offline message", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: false,
          onEvent: () => {},
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { wrapper: createFactoryEventStreamTestWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(0);
    expect(useDashboardStreamStore.getState().streamState).toMatchObject({
      status: "offline",
      message: "Live session updates paused. Showing last event state.",
    });
  });

  it("reopens the stream after refreshToken changes", async () => {
    const { rerender } = renderHook(
      ({ refreshToken }: { refreshToken: number }) =>
        useFactoryEventStream({
          enabled: true,
          onEvent: () => {},
          refreshToken,
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      {
        initialProps: { refreshToken: 0 },
        wrapper: createFactoryEventStreamTestWrapper(queryClient),
      },
    );

    expect(replayHarness.getStreams()).toHaveLength(1);

    act(() => {
      rerender({ refreshToken: 1 });
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });
    expect(replayHarness.getStreams()[1]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
  });

  it("reopens the stream when resuming from pause without clearing onEvent delivery", async () => {
    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useFactoryEventStream({
          enabled,
          onEvent: (event) => {
            receivedEvents.push(event);
          },
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      {
        initialProps: { enabled: true },
        wrapper: createFactoryEventStreamTestWrapper(queryClient),
      },
    );

    expect(replayHarness.getStreams()).toHaveLength(1);

    act(() => {
      rerender({ enabled: false });
    });

    await waitFor(() => {
      expect(useDashboardStreamStore.getState().streamState.status).toBe(
        "offline",
      );
    });
    expect(replayHarness.getStreams()).toHaveLength(1);

    act(() => {
      rerender({ enabled: true });
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });
  });

  it("does not open a stream when sessionID is null", () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: false,
          onEvent: () => {},
          sessionID: null,
        }),
      { wrapper: createFactoryEventStreamTestWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(0);
  });
});

describe("useFactoryEventStream query side effects", () => {
  let queryClient = createFactoryEventStreamQueryClient();

  beforeEach(() => {
    replayHarness.install();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 200,
        }),
      ),
    );
    queryClient = createFactoryEventStreamQueryClient();
    seedFactoryEventStreamStores();
  });

  afterEach(() => {
    resetFactoryEventStreamStores();
    vi.unstubAllGlobals();
  });

  it("updates factory definition queries from FACTORY_CHANGE events without invalidating document reads", async () => {
    queryClient.setQueryData(CURRENT_FACTORY_DOCUMENT_QUERY_KEY, {
      name: "factory",
      workers: [],
      workstations: [],
      workTypes: [],
      version: {
        logical: "7",
        physical: "2026-05-17T14:59:00Z",
      },
    });

    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          onEvent: () => {},
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { wrapper: createFactoryEventStreamTestWrapper(queryClient) },
    );

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-05-17T15:00:00Z",
          sequence: 8,
          tick: 8,
        },
        id: "factory-event/factory-change/8",
        payload: {
          factory: {
            name: "factory",
            workers: [
              {
                model: "gpt-5.6",
                modelProvider: "CODEX",
                name: "reviewer",
                type: "MODEL_WORKER",
              },
            ],
            workTypes: [
              {
                name: "story",
                states: [{ name: "new", type: "INITIAL" }],
              },
            ],
            workstations: [
              {
                body: "Updated prompt",
                id: "review",
                inputs: [{ state: "new", workType: "story" }],
                name: "Review",
                outputs: [],
                promptFile: "prompts/review.md",
                worker: "reviewer",
              },
            ],
          },
        },
        type: FACTORY_EVENT_TYPES.factoryChange,
      });
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    await waitFor(() => {
      expect(
        queryClient.getQueryData(CURRENT_FACTORY_DEFINITION_QUERY_KEY),
      ).toMatchObject({
        workers: [expect.objectContaining({ model: "gpt-5.6" })],
      });
    });

    await waitFor(() => {
      expect(
        queryClient.getQueryState(CURRENT_FACTORY_DOCUMENT_QUERY_KEY)
          ?.isInvalidated,
      ).toBe(false);
    });
  });
});
