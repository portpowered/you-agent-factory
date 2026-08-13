import { afterEach, describe, expect, it } from "bun:test";
import { act, renderHook } from "@testing-library/react";

import type {
  OpenWorkerSessionEventStreamOptions,
  WorkerSessionEventFrame,
  WorkerSessionEventSourceLike,
} from "../../../api/worker-sessions";
import { useWorkerSessionTimeline } from "./useWorkerSessionTimeline";

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
    providerSession: { provider: "", kind: "", id: "" },
    workIds: [],
    workerSessionId: "worker-1",
    ...overrides,
  };
}

afterEach(() => {
  streams.length = 0;
  pendingReconnect = null;
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
