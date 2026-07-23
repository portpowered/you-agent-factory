import {
  awaitingReplaySessionID,
  buildAwaitingDurableSession,
  buildAwaitingReplayEventStream,
  buildEmptyReplayEventStream,
  buildSuccessfulDurableSession,
  buildSuccessfulReplayDispatchList,
  buildSuccessfulReplayEventStream,
  buildWarningDurableSession,
  buildWarningReplayDispatchList,
  buildWarningReplayEventStream,
  emptyReplaySessionID,
  errorReplaySessionID,
  successfulReplaySessionID,
  unavailableReplaySessionID,
  warningReplaySessionID,
} from "../../testing/factory-session-event-replay-fixtures";
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
        buildSuccessfulReplayEventStream(successfulReplaySessionID),
        {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const events = await listFactorySessionEventReplay(
      successfulReplaySessionID,
    );

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
      parseFactoryEventReplayStream(buildEmptyReplayEventStream()),
    ).toEqual([]);
  });

  it("rejects malformed Factory Event payloads", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('data: {"id":"evt-1"}\n\n', {
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

describe("factory session event replay shared fixtures", () => {
  it("parses warning and awaiting durable replay fixtures into ordered Factory Events", () => {
    expect(
      parseFactoryEventReplayStream(buildWarningReplayEventStream()),
    ).toEqual([
      expect.objectContaining({
        id: "evt-w1",
        type: "JAVASCRIPT_CHECKPOINT_REF",
      }),
      expect.objectContaining({
        id: "evt-w2",
        type: FACTORY_EVENT_TYPES.dispatchInterrupted,
      }),
      expect.objectContaining({
        id: "evt-w3",
        type: FACTORY_EVENT_TYPES.sessionCompleted,
      }),
    ]);

    expect(
      parseFactoryEventReplayStream(buildAwaitingReplayEventStream()),
    ).toEqual([
      expect.objectContaining({
        context: expect.objectContaining({
          sessionId: awaitingReplaySessionID,
        }),
        id: "session-started/dur-sess-js-awaiting-001",
        type: FACTORY_EVENT_TYPES.sessionStarted,
      }),
      expect.objectContaining({
        context: expect.objectContaining({
          sessionId: awaitingReplaySessionID,
        }),
        id: "session-result-updated/dur-sess-js-awaiting-001",
        type: FACTORY_EVENT_TYPES.sessionResultUpdated,
      }),
    ]);
  });

  it("keeps the shared durable session and dispatch fixtures aligned to their session ids", () => {
    expect(buildSuccessfulDurableSession()).toMatchObject({
      orchestratorKind: "JAVASCRIPT",
      progress: expect.objectContaining({ totalDispatches: 2 }),
      sessionId: successfulReplaySessionID,
      status: "SUCCEEDED",
    });
    expect(buildWarningDurableSession()).toMatchObject({
      orchestratorKind: "JAVASCRIPT",
      progress: expect.objectContaining({ failedDispatches: 1 }),
      sessionId: warningReplaySessionID,
      status: "FAILED",
    });
    expect(buildAwaitingDurableSession()).toMatchObject({
      orchestratorKind: "JAVASCRIPT",
      resultSummary: expect.objectContaining({ resultStatus: "NOT_READY" }),
      sessionId: awaitingReplaySessionID,
      status: "AWAITING_APPROVAL",
    });
    expect(buildSuccessfulDurableSession(emptyReplaySessionID)).toMatchObject({
      sessionId: emptyReplaySessionID,
      status: "SUCCEEDED",
    });
    expect(buildWarningDurableSession(errorReplaySessionID)).toMatchObject({
      sessionId: errorReplaySessionID,
      status: "FAILED",
    });
    expect(
      buildWarningDurableSession(unavailableReplaySessionID),
    ).toMatchObject({
      sessionId: unavailableReplaySessionID,
      status: "FAILED",
    });
    expect(buildSuccessfulReplayDispatchList()).toEqual({
      dispatches: [
        expect.objectContaining({
          id: "dispatch-1",
          phase: "review",
          status: "COMPLETED",
        }),
      ],
      sessionId: successfulReplaySessionID,
    });
    expect(buildWarningReplayDispatchList()).toEqual({
      dispatches: [
        expect.objectContaining({
          id: "dispatch-verify",
          phase: "verify",
          status: "FAILED",
        }),
      ],
      sessionId: warningReplaySessionID,
    });
  });
});
