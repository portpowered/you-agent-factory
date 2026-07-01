import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";
import { useFactoryEventStream } from "./useFactoryEventStream";
import { CANONICAL_SELECTED_TICK_EVENTS } from "./useFactoryEventStream.fixtures";
import {
  createFactoryEventStreamQueryClient,
  createFactoryEventStreamTestWrapper,
  replayHarness,
  resetFactoryEventStreamStores,
  seedFactoryEventStreamStores,
} from "./useFactoryEventStream.test-support";

describe("useFactoryEventStream generic reconnect", () => {
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

  it("surfaces offline stream state when the connection fails before the first event", async () => {
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

    act(() => {
      stream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(useDashboardStreamStore.getState().streamState).toMatchObject({
        status: "offline",
        message: "Factory event stream disconnected. Showing last event state.",
      });
    });
  });

  it("reconnects from the last delivered event after a stream error", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          locale: "en",
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
      stream.emit("message", CANONICAL_SELECTED_TICK_EVENTS[0]);
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    act(() => {
      stream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(useDashboardStreamStore.getState().streamState).toMatchObject({
        message: "Reconnecting event stream",
        status: "reconnecting",
      });
    });

    await waitFor(
      () => {
        expect(replayHarness.getStreams()).toHaveLength(2);
      },
      { timeout: 3000 },
    );
    expect(replayHarness.getStreams()[1]?.url).toContain(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
  });

  it("clears the stale reconnect cursor and retries once from scratch", async () => {
    const onInvalidReconnectCursor = vi.fn();
    const validateReconnectCursor = vi
      .fn()
      .mockResolvedValueOnce({
        message:
          "Factory event replay cursor no longer matches the current session history.",
        ok: false,
        reason: "stale_cursor",
      })
      .mockResolvedValueOnce({ ok: true });

    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          initialReconnectCursor: {
            afterEventId: "checkpoint-event-7",
            afterSequence: 7,
          },
          onEvent: () => {},
          onInvalidReconnectCursor,
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          validateReconnectCursor,
        }),
      { wrapper: createFactoryEventStreamTestWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(onInvalidReconnectCursor).toHaveBeenCalledTimes(1);
    expect(validateReconnectCursor).toHaveBeenNthCalledWith(
      1,
      DEFAULT_FACTORY_SESSION_ID,
      {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
      },
    );
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
  });
});
