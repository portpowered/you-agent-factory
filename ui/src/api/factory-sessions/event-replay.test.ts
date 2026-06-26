import { FACTORY_EVENT_TYPES } from "../events";
import {
  listFactorySessionEventReplay,
  parseFactoryEventReplayStream,
} from "./event-replay";

describe("factory session event replay API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("parses a durable factory session replay stream into ordered Factory Events", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        [
          "event: message",
          'data: {"id":"evt-1","type":"SESSION_STARTED","context":{"sequence":1,"tick":1,"eventTime":"2026-06-25T10:00:00Z","sessionId":"dur-sess-js-success-002","sessionSequence":1},"payload":{"startedAt":"2026-06-25T10:00:00Z"}}',
          "",
          'data: {"id":"evt-2","type":"DISPATCH_QUEUED","context":{"sequence":2,"tick":2,"eventTime":"2026-06-25T10:00:02Z","sessionId":"dur-sess-js-success-002","sessionSequence":2,"dispatchId":"dispatch-1"},"payload":{"dispatchKind":"JAVASCRIPT_AGENT"}}',
          "",
        ].join("\n"),
        {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listFactorySessionEventReplay("dur-sess-js-success-002"),
    ).resolves.toEqual([
      expect.objectContaining({
        id: "evt-1",
        type: FACTORY_EVENT_TYPES.sessionStarted,
      }),
      expect.objectContaining({
        id: "evt-2",
        type: FACTORY_EVENT_TYPES.dispatchQueued,
      }),
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-success-002/events",
      expect.objectContaining({
        headers: { Accept: "text/event-stream" },
        method: "GET",
      }),
    );
  });

  it("skips non-message SSE blocks and returns an empty event list for sparse history", () => {
    expect(
      parseFactoryEventReplayStream(
        [
          ": keepalive",
          "",
          "event: ping",
          "data: ignored",
          "",
        ].join("\n"),
      ),
    ).toEqual([]);
  });

  it("rejects malformed Factory Event payloads", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("data: {\"id\":\"evt-1\"}\n\n", {
        headers: {
          "Content-Type": "text/event-stream",
        },
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listFactorySessionEventReplay("dur-sess-js-success-002"),
    ).rejects.toMatchObject({
      message: "The factory sessions API returned an invalid response.",
    });
  });
});
