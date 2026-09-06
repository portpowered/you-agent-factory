import { describe, expect, it } from "bun:test";

import type { WorkerSessionEventFrame } from "../../../api/worker-sessions";
import type { WorkerSessionEventRecord } from "./worker-session-timeline-projection-types";
import {
  applyWorkerSessionTimelineFrame,
  applyWorkerSessionTimelineTransportFailure,
  createWorkerSessionTimelineStreamState,
  type WorkerSessionTimelineStreamContext,
} from "./worker-session-timeline-stream";

const context: WorkerSessionTimelineStreamContext = {
  workerSessionID: "worker-1",
};

function record(
  position: number,
  overrides: Partial<WorkerSessionEventRecord> = {},
): WorkerSessionEventRecord {
  return {
    cursor: {
      position,
      workerSessionId: "worker-1",
      streamGenerationId: "generation-1",
    },
    payload: {},
    position,
    schemaId: "workers.draft.v1",
    sourceEventId: `event-${position}`,
    sourceId: "worker-1",
    sourceSequence: position,
    sourceType: "worker_session",
    ...overrides,
  };
}

function frame(
  delivery: WorkerSessionEventFrame["delivery"],
  event: WorkerSessionEventRecord | null,
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

function apply(
  state: ReturnType<typeof createWorkerSessionTimelineStreamState>,
  nextFrame: WorkerSessionEventFrame,
) {
  return applyWorkerSessionTimelineFrame(state, nextFrame, context);
}

describe("Worker Session timeline stream state", () => {
  it("accepts an ordered retained-to-live sequence and ignores exact overlap", () => {
    const first = apply(
      createWorkerSessionTimelineStreamState(),
      frame("RECORD", record(1)),
    );
    const second = apply(first.state, frame("RECORD", record(2)));
    const duplicate = apply(second.state, frame("RECORD", record(2)));

    expect(duplicate.state.records.map(({ position }) => position)).toEqual([
      1, 2,
    ]);
    expect(duplicate.state.acknowledgedCursor?.position).toBe(2);
    expect(duplicate.shouldStop).toBe(false);
  });

  it("rejects gaps, foreign cursors, and generation changes visibly", () => {
    const first = apply(
      createWorkerSessionTimelineStreamState(),
      frame("RECORD", record(1)),
    ).state;
    const gap = apply(first, frame("RECORD", record(3)));
    expect(gap.state.sourceError).toMatchObject({ code: "CURSOR_GAP" });

    const foreign = apply(
      first,
      frame(
        "RECORD",
        record(2, { cursor: { position: 2, workerSessionId: "worker-2" } }),
      ),
    );
    expect(foreign.state.sourceError).toMatchObject({ code: "CURSOR_FOREIGN" });

    const generation = apply(
      first,
      frame(
        "RECORD",
        record(2, {
          cursor: {
            position: 2,
            workerSessionId: "worker-1",
            streamGenerationId: "generation-2",
          },
        }),
      ),
    );
    expect(generation.state.sourceError).toMatchObject({
      code: "CURSOR_GENERATION_MISMATCH",
    });
  });

  it("preserves records on source failure and carries replay health", () => {
    const first = apply(
      createWorkerSessionTimelineStreamState(),
      frame("RECORD", record(1), {
        recordingHealth: "DEGRADED",
        recordingHealthReason: "PERSISTENCE_FAILED",
      }),
    );
    const failure = apply(
      first.state,
      frame("SOURCE_FAILURE", null, {
        errorCode: "WORKER_SESSION_STREAM_UNAVAILABLE",
        errorMessage: "safe source failure",
      }),
    );

    expect(failure.state.records).toHaveLength(1);
    expect(failure.state.status).toBe("source-error");
    expect(failure.state.sourceError?.message).toBe("safe source failure");
    expect(failure.state.recordingHealth).toBe("DEGRADED");
    expect(failure.shouldStop).toBe(true);
  });

  it("stops after terminal delivery and finite replay summary", () => {
    const terminal = apply(
      createWorkerSessionTimelineStreamState(),
      frame("TERMINAL", record(1)),
    );
    expect(terminal.state.status).toBe("completed");
    expect(terminal.state.terminalDelivery).toBe("TERMINAL");
    expect(
      applyWorkerSessionTimelineTransportFailure(terminal.state),
    ).toMatchObject({ shouldReconnect: false });

    const summary = apply(
      createWorkerSessionTimelineStreamState(),
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
    expect(summary.state.status).toBe("ready-empty");
    expect(summary.state.recordingHealth).toBe("INCOMPLETE");
    expect(summary.state.replaySummary?.complete).toBe(false);
    expect(summary.shouldStop).toBe(true);
  });

  it("reconnects from the initial generation-scoped cursor", () => {
    const state = createWorkerSessionTimelineStreamState({
      afterPosition: 4,
      streamGenerationId: "generation-1",
    });
    const next = apply(state, frame("RECORD", record(5)));

    expect(next.state.acknowledgedCursor).toEqual({
      position: 5,
      workerSessionId: "worker-1",
      streamGenerationId: "generation-1",
    });
  });
});
