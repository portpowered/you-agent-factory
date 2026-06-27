import { factoryAPIURL } from "../baseUrl";
import { factorySessionScopedPath } from "../session-routing";
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

export interface FactoryEventReconnectCursor {
  afterEventId?: string;
  afterSequence?: number;
}

export type FactoryEventReconnectValidationResult =
  | { ok: true }
  | {
      message: string;
      ok: false;
      reason: "stale_cursor" | "unavailable";
    };

export type FactoryEventStreamStatus =
  | "connecting"
  | "live"
  | "offline"
  | "reconnecting";

function buildFactoryEventStreamURL(
  sessionID?: string | null,
  reconnect?: FactoryEventReconnectCursor,
): string {
  const basePath = factorySessionScopedPath(FACTORY_EVENTS_ENDPOINT, sessionID);
  const params = new URLSearchParams();
  if (reconnect?.afterEventId) {
    params.set("after_event_id", reconnect.afterEventId);
  }
  if (reconnect?.afterSequence != null) {
    params.set("after_sequence", String(reconnect.afterSequence));
  }
  const query = params.toString();
  return factoryAPIURL(query ? `${basePath}?${query}` : basePath);
}

export async function validateFactoryEventReconnectCursor(
  sessionID?: string | null,
  reconnect?: FactoryEventReconnectCursor,
  fetchImpl: typeof fetch = fetch,
): Promise<FactoryEventReconnectValidationResult> {
  if (reconnect == null) {
    return { ok: true };
  }

  try {
    const response = await fetchImpl(
      buildFactoryEventStreamURL(sessionID, reconnect),
      {
        headers: {
          Accept: "text/event-stream",
        },
      },
    );
    void response.body?.cancel().catch(() => {});
    if (response.ok) {
      return { ok: true };
    }
    if (response.status === 400) {
      // hardcoded-ui-copy-exception: non-product-diagnostic
      const message =
        "Factory event replay cursor no longer matches the current session history.";
      return {
        message,
        ok: false,
        reason: "stale_cursor",
      };
    }
    return {
      // hardcoded-ui-copy-exception: non-product-diagnostic
      message: "Factory event stream unavailable.",
      ok: false,
      reason: "unavailable",
    };
  } catch {
    return { ok: true };
  }
}

export function openFactoryEventStream(
  onEvent: (event: FactoryEvent) => void,
  onStatusChange: (
    status: FactoryEventStreamStatus,
    message: string,
  ) => void,
  sessionID?: string | null,
  reconnect?: FactoryEventReconnectCursor,
): EventSourceLike | null {
  const EventSourceImpl = factoryEventSource();
  if (EventSourceImpl === null) {
    onStatusChange("offline", "Factory events unavailable in this browser.");
    return null;
  }

  const reconnecting = reconnect != null;
  const stream = new EventSourceImpl(
    buildFactoryEventStreamURL(sessionID, reconnect),
  );
  onStatusChange(
    reconnecting ? "reconnecting" : "connecting",
    reconnecting
      ? "Reconnecting to factory events..."
      : "Connecting to factory events...",
  );
  stream.onopen = () => {
    onStatusChange("live", "Factory event stream connected.");
  };
  stream.onerror = () => {
    onStatusChange(
      "offline",
      "Factory event stream disconnected. Showing last event state.",
    );
  };
  stream.addEventListener("message", (event) => {
    if (event instanceof MessageEvent) {
      onEvent(JSON.parse(event.data) as FactoryEvent);
    }
  });
  return stream;
}
