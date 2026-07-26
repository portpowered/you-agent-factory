import {
  openFactoryEventStream,
  probeFactoryEventStreamRecovery,
  validateFactoryEventReconnectCursor,
} from "./api";
import type { FactoryEvent } from "./types";

class MockEventSource {
  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;
  public readonly listeners = new Map<string, EventListener>();

  public constructor(public readonly url: string) {}

  public addEventListener(type: string, listener: EventListener): void {
    this.listeners.set(type, listener);
  }

  public close(): void {}
}

describe("factory events stream API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reports an offline state when the browser does not support EventSource", () => {
    vi.stubGlobal("window", {
      ...window,
      EventSource: undefined,
    });
    const onEvent = vi.fn();
    const onStatusChange = vi.fn();

    const stream = openFactoryEventStream(onEvent, onStatusChange);

    expect(stream).toBeNull();
    expect(onEvent).not.toHaveBeenCalled();
    expect(onStatusChange).toHaveBeenCalledWith(
      "offline",
      "Factory events unavailable in this browser.",
    );
  });

  it("opens the explicit default-session event stream and translates lifecycle plus message events", () => {
    const onEvent = vi.fn();
    const onStatusChange = vi.fn();
    vi.stubGlobal("EventSource", MockEventSource);

    const stream = openFactoryEventStream(onEvent, onStatusChange);

    expect(stream).toBeInstanceOf(MockEventSource);
    expect(stream?.url).toBe("/factory-sessions/~default/events");
    expect(onStatusChange).toHaveBeenCalledWith(
      "connecting",
      "Connecting to factory events...",
    );

    stream?.onopen?.(new Event("open"));
    expect(onStatusChange).toHaveBeenCalledWith(
      "live",
      "Factory event stream connected.",
    );

    const messageListener = stream?.listeners.get("message");
    const eventPayload = {
      created_at: "2026-05-05T20:00:00Z",
      event: "FACTORY_SNAPSHOT",
      sequence: 1,
    } satisfies Partial<FactoryEvent>;
    messageListener?.(
      new MessageEvent("message", {
        data: JSON.stringify(eventPayload),
      }),
    );
    expect(onEvent).toHaveBeenCalledWith(eventPayload);

    messageListener?.(new Event("message"));
    expect(onEvent).toHaveBeenCalledTimes(1);

    stream?.onerror?.(new Event("error"));
    expect(onStatusChange).toHaveBeenCalledWith(
      "offline",
      "Factory event stream disconnected. Showing last event state.",
    );
  });

  it("appends reconnect query parameters when a cursor is provided", () => {
    const onEvent = vi.fn();
    const onStatusChange = vi.fn();
    vi.stubGlobal("EventSource", MockEventSource);

    const stream = openFactoryEventStream(
      onEvent,
      onStatusChange,
      "session-beta",
      { afterEventId: "event-3", afterSequence: 12 },
    );

    expect(stream?.url).toBe(
      "/factory-sessions/session-beta/events?after_event_id=event-3&after_sequence=12",
    );
    expect(onStatusChange).toHaveBeenCalledWith(
      "reconnecting",
      "Reconnecting to factory events...",
    );
  });

  it("opens the session-scoped event stream when a non-default session is selected", () => {
    const onEvent = vi.fn();
    const onStatusChange = vi.fn();
    vi.stubGlobal("EventSource", MockEventSource);

    const stream = openFactoryEventStream(
      onEvent,
      onStatusChange,
      "session-beta",
    );

    expect(stream).toBeInstanceOf(MockEventSource);
    expect(stream?.url).toBe("/factory-sessions/session-beta/events");
  });
});

describe("factory events recovery probe API", () => {
  it("probes session event recovery as structured JSON", async () => {
    const fetchImplementation = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          factorySessionId: "session-beta",
          outcome: "CURSOR_STALE",
          retry: {
            omitAfterEventId: true,
            omitAfterSequence: true,
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );

    await expect(
      probeFactoryEventStreamRecovery(
        "session-beta",
        { afterEventId: "event-3", afterSequence: 12 },
        { fetch: fetchImplementation },
      ),
    ).resolves.toEqual({
      factorySessionId: "session-beta",
      outcome: "CURSOR_STALE",
      retry: {
        omitAfterEventId: true,
        omitAfterSequence: true,
      },
    });

    expect(fetchImplementation).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/events?after_event_id=event-3&after_sequence=12",
      {
        headers: {
          Accept: "application/json",
        },
        method: "GET",
      },
    );
  });

  it("surfaces probe transport failures with structured error details", async () => {
    const fetchImplementation = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "INTERNAL_ERROR",
          message: "probe failed",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 500,
          statusText: "Internal Server Error",
        },
      ),
    );

    await expect(
      probeFactoryEventStreamRecovery("session-beta", undefined, {
        fetch: fetchImplementation,
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "probe failed",
      status: 500,
      statusText: "Internal Server Error",
    });
  });
});

describe("factory events reconnect validation", () => {
  it("treats a 400 reconnect probe as a stale cursor", async () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      body: {
        cancel: vi.fn().mockResolvedValue(undefined),
      },
      ok: false,
      status: 400,
    });

    await expect(
      validateFactoryEventReconnectCursor(
        "session-beta",
        { afterEventId: "event-3", afterSequence: 12 },
        fetchSpy,
      ),
    ).resolves.toEqual({
      message:
        "Factory event replay cursor no longer matches the current session history.",
      ok: false,
      reason: "stale_cursor",
    });
    expect(fetchSpy).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/events?after_event_id=event-3&after_sequence=12",
      {
        headers: {
          Accept: "text/event-stream",
        },
      },
    );
  });

  it("treats a successful reconnect probe as reusable", async () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      body: {
        cancel: vi.fn().mockResolvedValue(undefined),
      },
      ok: true,
      status: 200,
    });

    await expect(
      validateFactoryEventReconnectCursor(
        "session-beta",
        { afterEventId: "event-3", afterSequence: 12 },
        fetchSpy,
      ),
    ).resolves.toEqual({ ok: true });
  });
});
// Component lane: requires DOM APIs.
