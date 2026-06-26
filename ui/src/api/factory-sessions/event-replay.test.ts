import { FACTORY_EVENT_TYPES } from "../events";
import {
  buildSuccessfulReplayEventStream,
  successfulReplaySessionID,
} from "../../testing/factory-session-event-replay-fixtures";
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
      new Response(buildSuccessfulReplayEventStream(successfulReplaySessionID), {
        headers: {
          "Content-Type": "text/event-stream",
        },
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const events = await listFactorySessionEventReplay(successfulReplaySessionID);

    expect(events).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "evt-1",
          type: FACTORY_EVENT_TYPES.sessionStarted,
        }),
        expect.objectContaining({
          id: "evt-2",
          type: FACTORY_EVENT_TYPES.orchestratorPhaseChanged,
        }),
        expect.objectContaining({
          id: "evt-3",
          type: FACTORY_EVENT_TYPES.dispatchQueued,
        }),
      ]),
    );
    expect(events).toHaveLength(5);
    expect(events[0]).toMatchObject({
      id: "evt-1",
      type: FACTORY_EVENT_TYPES.sessionStarted,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      `/factory-sessions/${successfulReplaySessionID}/events`,
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
      listFactorySessionEventReplay(successfulReplaySessionID),
    ).rejects.toMatchObject({
      message: "The factory sessions API returned an invalid response.",
    });
  });
});
