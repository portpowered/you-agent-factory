import { openFactoryEventStream } from "./api";
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

describe("factory events API", () => {
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

  it("opens the event stream and translates lifecycle plus message events", () => {
    const onEvent = vi.fn();
    const onStatusChange = vi.fn();
    vi.stubGlobal("EventSource", MockEventSource);

    const stream = openFactoryEventStream(onEvent, onStatusChange);

    expect(stream).toBeInstanceOf(MockEventSource);
    expect(stream?.url).toBe("/events");
    expect(onStatusChange).toHaveBeenCalledWith("connecting", "Connecting to factory events...");

    stream?.onopen?.(new Event("open"));
    expect(onStatusChange).toHaveBeenCalledWith("live", "Factory event stream connected.");

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
});
