import { factoryAPIURL } from "../baseUrl";
import type { FactoryEvent } from "./types";
import { FACTORY_EVENTS_ENDPOINT } from "./types";

export interface EventSourceLike {
  addEventListener: (type: string, listener: EventListener) => void;
  close: () => void;
  onerror: ((event: Event) => void) | null;
  onopen: ((event: Event) => void) | null;
}

type EventSourceCtor = new (url: string) => EventSourceLike;

function factoryEventSource(): EventSourceCtor | null {
  if (typeof window.EventSource === "undefined") {
    return null;
  }
  return window.EventSource as unknown as EventSourceCtor;
}

export function openFactoryEventStream(
  onEvent: (event: FactoryEvent) => void,
  onStatusChange: (status: "connecting" | "live" | "offline", message: string) => void,
): EventSourceLike | null {
  const EventSourceImpl = factoryEventSource();
  if (EventSourceImpl === null) {
    onStatusChange("offline", "Factory events unavailable in this browser.");
    return null;
  }

  const stream = new EventSourceImpl(factoryAPIURL(FACTORY_EVENTS_ENDPOINT));
  onStatusChange("connecting", "Connecting to factory events...");
  stream.onopen = () => {
    onStatusChange("live", "Factory event stream connected.");
  };
  stream.onerror = () => {
    onStatusChange("offline", "Factory event stream disconnected. Showing last event state.");
  };
  stream.addEventListener("message", (event) => {
    if (event instanceof MessageEvent) {
      onEvent(JSON.parse(event.data) as FactoryEvent);
    }
  });
  return stream;
}
