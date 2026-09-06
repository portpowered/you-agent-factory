import { describe, expect, it } from "bun:test";

import {
  buildWorkerSessionEventStreamURL,
  parseWorkerSessionEventFrame,
  WorkerSessionEventStreamParseError,
} from "./api";
import { listWorkerSessionsForWork } from "./observations";

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
      providerSession: null,
      recordingHealthReason: null,
      workIds: [],
      workerSessionId: "worker-1",
    });

    expect(frame.event).toBeNull();
    expect(frame.errorCode).toBe("WORKER_SESSION_STREAM_UNAVAILABLE");
    expect(frame.providerSession).toBeNull();
    expect(frame.recordingHealthReason).toBeNull();
  });

  it("normalizes an omitted provider identity to the explicit null state", () => {
    const frame = parseWorkerSessionEventFrame({
      delivery: "SOURCE_FAILURE",
      errorCode: "WORKER_SESSION_STREAM_UNAVAILABLE",
      errorMessage: "retained Worker Session history is unavailable",
      event: null,
      workIds: [],
      workerSessionId: "worker-1",
    });

    expect(frame.providerSession).toBeNull();
  });

  it("rejects malformed frames with a safe typed parse error", () => {
    expect(() =>
      parseWorkerSessionEventFrame({
        delivery: "RECORD",
        event: null,
        providerSession: null,
        workIds: [],
        workerSessionId: "worker-1",
      }),
    ).toThrowError(WorkerSessionEventStreamParseError);
  });
});

describe("Worker Session observation API", () => {
  it("lists observations for the selected Work in the selected Factory Session", async () => {
    const fetchImplementation = async (
      input: RequestInfo | URL,
      init?: RequestInit,
    ) => {
      expect(String(input)).toBe(
        "/factory-sessions/factory%2Fsession/worker-sessions?workId=work%2F1",
      );
      expect(init?.method).toBe("GET");
      return new Response(
        JSON.stringify({
          sessions: [
            {
              attemptId: "attempt-1",
              direct: false,
              durationBasis: "AUTHORITATIVE",
              durationMillis: null,
              endedAt: null,
              factorySessionId: "factory/session",
              parse: { errors: [], ignored: 0 },
              providerSessionAvailable: false,
              recordingHealth: "COMPLETE",
              startedAt: null,
              state: "RUNNING",
              transcript: "AVAILABLE",
              turnId: null,
              workIds: ["work/1"],
              workerSessionId: "worker-1",
            },
          ],
        }),
        { status: 200 },
      );
    };

    await expect(
      listWorkerSessionsForWork({
        factorySessionID: "factory/session",
        fetch: fetchImplementation,
        workID: "work/1",
      }),
    ).resolves.toMatchObject([{ workerSessionId: "worker-1" }]);
  });

  it("rejects an observation response that does not preserve the typed list shape", async () => {
    await expect(
      listWorkerSessionsForWork({
        factorySessionID: "factory-1",
        fetch: async () => new Response(JSON.stringify({ sessions: [{}] })),
        workID: "work-1",
      }),
    ).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
});
