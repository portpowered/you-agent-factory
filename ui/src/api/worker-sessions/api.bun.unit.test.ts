import { describe, expect, it } from "bun:test";

import {
  buildWorkerSessionEventStreamURL,
  parseWorkerSessionEventFrame,
  WorkerSessionEventStreamParseError,
} from "./api";

describe("Worker Session event stream API", () => {
  it("builds the provider-neutral scoped URL with a reconnect cursor", () => {
    expect(
      buildWorkerSessionEventStreamURL({
        factorySessionID: "factory/session",
        workerSessionID: "worker session",
        reconnect: {
          afterPosition: 17,
          streamGenerationId: "generation-1",
        },
        replayOnly: true,
      }),
    ).toBe(
      "/factory-sessions/factory%2Fsession/worker-sessions/worker%20session/events?replayOnly=true&after_position=17&stream_generation_id=generation-1",
    );
  });

  it("keeps source-failure frames typed without inventing a canonical record", () => {
    const frame = parseWorkerSessionEventFrame({
      delivery: "SOURCE_FAILURE",
      errorCode: "WORKER_SESSION_STREAM_UNAVAILABLE",
      errorMessage: "retained Worker Session history is unavailable",
      event: null,
      providerSession: { provider: "", kind: "", id: "" },
      workIds: [],
      workerSessionId: "worker-1",
    });

    expect(frame.event).toBeNull();
    expect(frame.errorCode).toBe("WORKER_SESSION_STREAM_UNAVAILABLE");
  });

  it("rejects malformed frames with a safe typed parse error", () => {
    expect(() =>
      parseWorkerSessionEventFrame({
        delivery: "RECORD",
        event: null,
        providerSession: { provider: "", kind: "", id: "" },
        workIds: [],
        workerSessionId: "worker-1",
      }),
    ).toThrowError(WorkerSessionEventStreamParseError);
  });
});
