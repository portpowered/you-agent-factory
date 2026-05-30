import { describe, expect, it } from "vitest";

import { deriveDashboardWorldViewShellState } from "./dashboard-world-view";

const CONNECTING_STREAM = {
  status: "connecting" as const,
  message: "Loading factory events...",
};

const OFFLINE_STREAM = {
  status: "offline" as const,
  message: "Factory event stream disconnected. Showing last event state.",
};

describe("deriveDashboardWorldViewShellState", () => {
  it("reports initial loading while a session is selected and the stream is still connecting", () => {
    expect(
      deriveDashboardWorldViewShellState({
        eventCount: 0,
        rawSessionID: "session-alpha",
        selectedTick: 0,
        streamState: CONNECTING_STREAM,
      }),
    ).toEqual({
      error: null,
      hasEvents: false,
      isInitialLoading: true,
    });
  });

  it("transitions from loading to error when the stream goes offline before the first event", () => {
    const loading = deriveDashboardWorldViewShellState({
      eventCount: 0,
      rawSessionID: "session-alpha",
      selectedTick: 0,
      streamState: CONNECTING_STREAM,
    });
    const errored = deriveDashboardWorldViewShellState({
      eventCount: 0,
      rawSessionID: "session-alpha",
      selectedTick: 0,
      streamState: OFFLINE_STREAM,
    });

    expect(loading.isInitialLoading).toBe(true);
    expect(loading.error).toBeNull();
    expect(errored.isInitialLoading).toBe(false);
    expect(errored.error?.message).toBe(OFFLINE_STREAM.message);
    expect(errored.hasEvents).toBe(false);
  });

  it("transitions from loading to success once streamed events arrive", () => {
    const loading = deriveDashboardWorldViewShellState({
      eventCount: 0,
      rawSessionID: "session-alpha",
      selectedTick: 0,
      streamState: CONNECTING_STREAM,
    });
    const ready = deriveDashboardWorldViewShellState({
      eventCount: 2,
      rawSessionID: "session-alpha",
      selectedTick: 2,
      streamState: CONNECTING_STREAM,
    });

    expect(loading.isInitialLoading).toBe(true);
    expect(ready.isInitialLoading).toBe(false);
    expect(ready.error).toBeNull();
    expect(ready.hasEvents).toBe(true);
  });

  it("does not show loading or error when no session is selected", () => {
    expect(
      deriveDashboardWorldViewShellState({
        eventCount: 0,
        rawSessionID: null,
        selectedTick: 0,
        streamState: OFFLINE_STREAM,
      }),
    ).toEqual({
      error: null,
      hasEvents: false,
      isInitialLoading: false,
    });
  });
});
