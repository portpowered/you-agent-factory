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

const RECOVERY_FAILED_STREAM = {
  status: "recovery_failed" as const,
  message: "The dashboard could not restore this session automatically.",
};

const LIVE_STREAM = {
  status: "live" as const,
  message: "Factory event stream connected.",
};

describe("deriveDashboardWorldViewShellState", () => {
  it("reports initial loading while a session is selected and the stream is still connecting", () => {
    expect(
      deriveDashboardWorldViewShellState({
        eventCount: 0,
        hasRestoredCheckpoint: false,
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
      hasRestoredCheckpoint: false,
      rawSessionID: "session-alpha",
      selectedTick: 0,
      streamState: CONNECTING_STREAM,
    });
    const errored = deriveDashboardWorldViewShellState({
      eventCount: 0,
      hasRestoredCheckpoint: false,
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
      hasRestoredCheckpoint: false,
      rawSessionID: "session-alpha",
      selectedTick: 0,
      streamState: CONNECTING_STREAM,
    });
    const ready = deriveDashboardWorldViewShellState({
      eventCount: 2,
      hasRestoredCheckpoint: false,
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
        hasRestoredCheckpoint: false,
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

  it("treats replay recovery failure as a shell error before the first event", () => {
    const errored = deriveDashboardWorldViewShellState({
      eventCount: 0,
      hasRestoredCheckpoint: false,
      rawSessionID: "session-alpha",
      selectedTick: 0,
      streamState: RECOVERY_FAILED_STREAM,
    });

    expect(errored.isInitialLoading).toBe(false);
    expect(errored.error?.message).toBe(RECOVERY_FAILED_STREAM.message);
    expect(errored.hasEvents).toBe(false);
  });

  it("treats a hydrated tick-zero checkpoint as current while the reopened stream is quiet", () => {
    const ready = deriveDashboardWorldViewShellState({
      eventCount: 0,
      hasRestoredCheckpoint: true,
      rawSessionID: "session-alpha",
      selectedTick: 0,
      streamState: LIVE_STREAM,
    });

    expect(ready).toEqual({
      error: null,
      hasEvents: false,
      isInitialLoading: false,
    });
  });

  it("treats a hydrated nonzero checkpoint as current while the reopened stream is quiet", () => {
    const ready = deriveDashboardWorldViewShellState({
      eventCount: 0,
      hasRestoredCheckpoint: true,
      rawSessionID: "session-alpha",
      selectedTick: 7,
      streamState: LIVE_STREAM,
    });

    expect(ready).toEqual({
      error: null,
      hasEvents: false,
      isInitialLoading: false,
    });
  });
});
