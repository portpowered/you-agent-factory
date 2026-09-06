import { afterEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type {
  OpenWorkerSessionEventStreamOptions,
  WorkerSessionEventFrame,
  WorkerSessionEventSourceLike,
  WorkerSessionObservation,
} from "../../../api/worker-sessions";
import { getWorkerSessionTimelineSelectionStorageKey } from "../lib/worker-session-timeline-selection";
import { useWorkerSessionTimeline } from "./useWorkerSessionTimeline";
import { useWorkerSessionTimelineTarget } from "./useWorkerSessionTimelineTarget";

class TestEventSource implements WorkerSessionEventSourceLike {
  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;
  public closed = false;

  public constructor(
    public readonly options: OpenWorkerSessionEventStreamOptions,
  ) {}

  public addEventListener(): void {}

  public close(): void {
    this.closed = true;
  }

  public emitFrame(frame: WorkerSessionEventFrame): void {
    this.options.onFrame(frame);
  }

  public emitOpen(): void {
    this.options.onStatusChange("live", "connected");
  }

  public emitOffline(message = "disconnected"): void {
    this.options.onStatusChange("offline", message);
  }
}

const streams: TestEventSource[] = [];
let pendingReconnect: (() => void) | null = null;

function openStream(
  options: OpenWorkerSessionEventStreamOptions,
): WorkerSessionEventSourceLike {
  const stream = new TestEventSource(options);
  streams.push(stream);
  options.onStatusChange("connecting", "connecting");
  return stream;
}

function scheduleReconnect(callback: () => void): () => void {
  pendingReconnect = callback;
  return () => {
    if (pendingReconnect === callback) {
      pendingReconnect = null;
    }
  };
}

function record(position: number, workerSessionID = "worker-1") {
  return {
    cursor: {
      position,
      workerSessionId: workerSessionID,
      streamGenerationId: "generation-1",
    },
    payload: {},
    position,
    schemaId: "workers.draft.v1",
    sourceEventId: `event-${position}`,
    sourceId: "worker-1",
    sourceSequence: position,
    sourceType: "worker_session",
  } as const;
}

function frame(
  delivery: WorkerSessionEventFrame["delivery"],
  event: WorkerSessionEventFrame["event"],
  overrides: Partial<WorkerSessionEventFrame> = {},
): WorkerSessionEventFrame {
  return {
    delivery,
    errorCode: null,
    errorMessage: null,
    event,
    providerSession: null,
    workIds: [],
    workerSessionId: "worker-1",
    ...overrides,
  };
}

afterEach(() => {
  streams.length = 0;
  pendingReconnect = null;
  window.sessionStorage.clear();
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: stream cases share one deterministic source harness.
describe("useWorkerSessionTimeline", () => {
  it("keeps retained and live records in one ordered deduplicated stream", () => {
    const { result } = renderHook(() =>
      useWorkerSessionTimeline({
        factorySessionID: "factory-1",
        openStream,
        workerSessionID: "worker-1",
      }),
    );
    const stream = streams[0];
    if (!stream) {
      throw new Error("expected Worker Session stream");
    }

    act(() => {
      stream.emitOpen();
      stream.emitFrame(frame("RECORD", record(1)));
      stream.emitFrame(frame("RECORD", record(2)));
      stream.emitFrame(frame("RECORD", record(2)));
    });

    expect(result.current.status).toBe("live");
    expect(result.current.records.map(({ position }) => position)).toEqual([
      1, 2,
    ]);
    expect(result.current.entries).toHaveLength(2);
    expect(result.current.reconnectCursor).toEqual({
      afterPosition: 2,
      streamGenerationId: "generation-1",
    });
  });

  it("reconnects only from the last acknowledged Worker Session cursor", () => {
    const { result } = renderHook(() =>
      useWorkerSessionTimeline({
        factorySessionID: "factory-1",
        openStream,
        reconnectDelayMs: 0,
        scheduleReconnect,
        workerSessionID: "worker-1",
      }),
    );
    const firstStream = streams[0];
    if (!firstStream) {
      throw new Error("expected initial Worker Session stream");
    }

    act(() => {
      firstStream.emitFrame(frame("RECORD", record(1)));
      firstStream.emitOffline();
    });

    expect(result.current.status).toBe("reconnecting");
    expect(pendingReconnect).not.toBeNull();
    act(() => {
      pendingReconnect?.();
    });

    expect(streams).toHaveLength(2);
    expect(streams[1]?.options.reconnect).toEqual({
      afterPosition: 1,
      streamGenerationId: "generation-1",
    });
  });

  it("preserves records and exposes a safe source failure", () => {
    const { result } = renderHook(() =>
      useWorkerSessionTimeline({
        factorySessionID: "factory-1",
        openStream,
        workerSessionID: "worker-1",
      }),
    );
    const stream = streams[0];
    if (!stream) {
      throw new Error("expected Worker Session stream");
    }

    act(() => {
      stream.emitFrame(frame("RECORD", record(1)));
      stream.emitFrame(
        frame("SOURCE_FAILURE", null, {
          errorCode: "WORKER_SESSION_STREAM_UNAVAILABLE",
          errorMessage: "history source is unavailable",
        }),
      );
    });

    expect(result.current.status).toBe("source-error");
    expect(result.current.records).toHaveLength(1);
    expect(result.current.sourceError).toEqual({
      code: "SOURCE_FAILURE",
      message: "history source is unavailable",
    });
    expect(pendingReconnect).toBeNull();
  });

  it("does not reconnect after terminal delivery or finite replay summary", () => {
    const terminal = renderHook(() =>
      useWorkerSessionTimeline({
        factorySessionID: "factory-1",
        openStream,
        scheduleReconnect,
        workerSessionID: "worker-1",
      }),
    );
    const terminalStream = streams[0];
    if (!terminalStream) {
      throw new Error("expected terminal Worker Session stream");
    }
    act(() => {
      terminalStream.emitFrame(frame("TERMINAL", record(1)));
      terminalStream.emitOffline();
    });
    expect(terminal.result.current.status).toBe("completed");
    expect(pendingReconnect).toBeNull();

    terminal.unmount();
    streams.length = 0;
    const replay = renderHook(() =>
      useWorkerSessionTimeline({
        factorySessionID: "factory-1",
        openStream,
        replayOnly: true,
        workerSessionID: "worker-1",
      }),
    );
    const replayStream = streams[0];
    if (!replayStream) {
      throw new Error("expected replay Worker Session stream");
    }
    act(() => {
      replayStream.emitFrame(
        frame("REPLAY_SUMMARY", null, {
          recordingHealth: "INCOMPLETE",
          recordingHealthReason: "RETAINED_HEAD_MOVED",
          replaySummary: {
            kind: "replay-summary",
            complete: false,
            reason: "ACTIVE",
            eventsEmitted: 0,
          },
        }),
      );
      replayStream.emitOffline();
    });
    expect(replay.result.current.status).toBe("ready-empty");
    expect(replay.result.current.recordingHealth).toBe("INCOMPLETE");
    expect(replay.result.current.replaySummary?.reason).toBe("ACTIVE");
    expect(pendingReconnect).toBeNull();
  });

  it("surfaces a cursor scope failure instead of joining another history", () => {
    const { result } = renderHook(() =>
      useWorkerSessionTimeline({
        factorySessionID: "factory-1",
        openStream,
        workerSessionID: "worker-1",
      }),
    );
    const stream = streams[0];
    if (!stream) {
      throw new Error("expected Worker Session stream");
    }
    act(() => {
      stream.emitFrame(frame("RECORD", record(1)));
      stream.emitFrame(frame("RECORD", record(2, "worker-2")));
    });

    expect(result.current.status).toBe("source-error");
    expect(result.current.sourceError?.code).toBe("CURSOR_FOREIGN");
    expect(result.current.records).toHaveLength(1);
  });
});

describe("useWorkerSessionTimelineTarget", () => {
  it("loads Worker Session targets for the selected Work and selects the first server-ordered attempt", async () => {
    const loadTargets = vi
      .fn()
      .mockResolvedValue([
        observation("worker-1", "attempt-1"),
        observation("worker-2", "attempt-2"),
      ]);

    const { result } = renderHook(
      () =>
        useWorkerSessionTimelineTarget({
          factorySessionID: "factory-1",
          loadTargets,
          workID: "work-1",
        }),
      { wrapper: createQueryWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("ready");
    });

    expect(loadTargets).toHaveBeenCalledWith(
      expect.objectContaining({
        factorySessionID: "factory-1",
        workID: "work-1",
      }),
    );
    expect(result.current.selectedWorkerSessionID).toBe("worker-1");

    act(() => {
      result.current.setSelectedWorkerSessionID("worker-2");
    });
    expect(result.current.selectedWorkerSessionID).toBe("worker-2");
    const selectionKey = getWorkerSessionTimelineSelectionStorageKey(
      "factory-1",
      "work-1",
    );
    expect(selectionKey).not.toBeNull();
    expect(window.sessionStorage.getItem(selectionKey as string)).toBe(
      "worker-2",
    );
  });

  it("restores a valid selection after remount and scopes it to Factory Session and Work", async () => {
    const selectionKey = getWorkerSessionTimelineSelectionStorageKey(
      "factory-1",
      "work-1",
    );
    if (selectionKey === null) {
      throw new Error("expected a scoped selection key");
    }
    window.sessionStorage.setItem(selectionKey, "worker-2");
    const loadTargets = vi
      .fn()
      .mockResolvedValue([
        observation("worker-1", "attempt-1"),
        observation("worker-2", "attempt-2"),
      ]);

    const first = renderHook(
      () =>
        useWorkerSessionTimelineTarget({
          factorySessionID: "factory-1",
          loadTargets,
          workID: "work-1",
        }),
      { wrapper: createQueryWrapper() },
    );
    await waitFor(() => {
      expect(first.result.current.selectedWorkerSessionID).toBe("worker-2");
    });
    first.unmount();

    const second = renderHook(
      () =>
        useWorkerSessionTimelineTarget({
          factorySessionID: "factory-1",
          loadTargets,
          workID: "work-1",
        }),
      { wrapper: createQueryWrapper() },
    );
    await waitFor(() => {
      expect(second.result.current.selectedWorkerSessionID).toBe("worker-2");
    });
    second.unmount();

    const otherScope = renderHook(
      () =>
        useWorkerSessionTimelineTarget({
          factorySessionID: "factory-1",
          loadTargets,
          workID: "work-2",
        }),
      { wrapper: createQueryWrapper() },
    );
    await waitFor(() => {
      expect(otherScope.result.current.selectedWorkerSessionID).toBe(
        "worker-1",
      );
    });
  });

  it("does not query until both the Factory Session and Work are selected", () => {
    const loadTargets = vi.fn();

    const { result } = renderHook(
      () =>
        useWorkerSessionTimelineTarget({
          factorySessionID: null,
          loadTargets,
          workID: null,
        }),
      { wrapper: createQueryWrapper() },
    );

    expect(result.current.status).toBe("idle");
    expect(loadTargets).not.toHaveBeenCalled();
  });
});

function createQueryWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function observation(
  workerSessionId: string,
  attemptId: string,
): WorkerSessionObservation {
  return {
    attemptId,
    direct: false,
    durationBasis: "UNAVAILABLE",
    durationMillis: null,
    endedAt: null,
    parse: { errors: [], ignored: 0 },
    providerSessionAvailable: false,
    startedAt: null,
    state: "RUNNING",
    transcript: "AVAILABLE",
    turnId: null,
    workIds: ["work-1"],
    workerSessionId,
  } as WorkerSessionObservation;
}
