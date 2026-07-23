import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import { factorySessionScopedPath } from "../session-routing";
import { extractAPIErrorPayload, readAPIResponseBody } from "../transport";
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

export type FactorySessionEventStreamRecovery =
  components["schemas"]["FactorySessionEventStreamRecovery"];

export interface ProbeFactoryEventStreamRecoveryOptions {
  fetch?: typeof globalThis.fetch;
}

export type FactoryEventStreamRecoveryProbeErrorCode =
  | "BAD_REQUEST"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export class FactoryEventStreamRecoveryProbeError extends Error {
  public readonly code: FactoryEventStreamRecoveryProbeErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: {
      code: FactoryEventStreamRecoveryProbeErrorCode;
      responseBody?: unknown;
      status?: number;
      statusText?: string;
    },
  ) {
    super(message);
    this.name = "FactoryEventStreamRecoveryProbeError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
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

function isFactorySessionEventStreamRecovery(
  value: unknown,
): value is FactorySessionEventStreamRecovery {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }

  const record = value as Record<string, unknown>;
  const retry = record.retry;
  if (typeof retry !== "object" || retry === null || Array.isArray(retry)) {
    return false;
  }

  const retryRecord = retry as Record<string, unknown>;
  return (
    typeof record.factorySessionId === "string" &&
    typeof record.outcome === "string" &&
    typeof retryRecord.omitAfterEventId === "boolean" &&
    typeof retryRecord.omitAfterSequence === "boolean"
  );
}

export async function probeFactoryEventStreamRecovery(
  sessionID?: string | null,
  reconnect?: FactoryEventReconnectCursor,
  options: ProbeFactoryEventStreamRecoveryOptions = {},
): Promise<FactorySessionEventStreamRecovery> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new FactoryEventStreamRecoveryProbeError(
      "Factory event recovery probing is unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      buildFactoryEventStreamURL(sessionID, reconnect),
      {
        headers: {
          Accept: "application/json",
        },
        method: "GET",
      },
    );
  } catch (error) {
    throw new FactoryEventStreamRecoveryProbeError(
      "The dashboard could not probe factory event recovery.",
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorPayload = extractAPIErrorPayload(responseBody);
    throw new FactoryEventStreamRecoveryProbeError(
      errorPayload?.message ?? "The factory event recovery probe failed.",
      {
        code:
          errorPayload?.code === "BAD_REQUEST" ||
          errorPayload?.code === "INTERNAL_ERROR"
            ? errorPayload.code
            : "INTERNAL_ERROR",
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isFactorySessionEventStreamRecovery(responseBody)) {
    throw new FactoryEventStreamRecoveryProbeError(
      "The factory event recovery probe returned an invalid response.",
      {
        code: "INTERNAL_ERROR",
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  return responseBody;
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
  onStatusChange: (status: FactoryEventStreamStatus, message: string) => void,
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
