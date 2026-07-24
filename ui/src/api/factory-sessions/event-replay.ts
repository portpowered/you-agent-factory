import { factoryAPIURL } from "../baseUrl";
import { FACTORY_EVENTS_ENDPOINT, type FactoryEvent } from "../events";
import { factorySessionScopedPath } from "../session-routing";
import { readAPIResponseBody } from "../transport";
import { buildFactorySessionsAPIError, FactorySessionsAPIError } from "./api";

export interface ListFactorySessionEventReplayOptions {
  fetch?: typeof globalThis.fetch;
}

export async function listFactorySessionEventReplay(
  sessionID: string,
  options: ListFactorySessionEventReplayOptions = {},
): Promise<FactoryEvent[]> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new FactorySessionsAPIError(
      "Factory sessions are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(
        factorySessionScopedPath(FACTORY_EVENTS_ENDPOINT, sessionID),
      ),
      {
        headers: {
          Accept: "text/event-stream",
        },
        method: "GET",
      },
    );
  } catch (error) {
    throw new FactorySessionsAPIError(
      "The dashboard could not reach the factory sessions API.",
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  if (!response.ok) {
    const responseBody = await readAPIResponseBody(response);
    throw buildFactorySessionsAPIError(
      response,
      responseBody,
      "The factory sessions API rejected the request.",
    );
  }

  const replayStream = await response.text();
  try {
    return parseFactoryEventReplayStream(replayStream);
  } catch (error) {
    throw new FactorySessionsAPIError(
      "The factory sessions API returned an invalid response.",
      {
        code: "INTERNAL_ERROR",
        responseBody: error,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }
}

export function parseFactoryEventReplayStream(
  replayStream: string,
): FactoryEvent[] {
  const events: FactoryEvent[] = [];
  const normalized = replayStream.replace(/\r\n/g, "\n");
  for (const block of normalized.split("\n\n")) {
    const parsed = parseReplayEventBlock(block);
    if (parsed) {
      events.push(parsed);
    }
  }
  return events;
}

function parseReplayEventBlock(block: string): FactoryEvent | null {
  const lines = block
    .split("\n")
    .map((line) => line.trimEnd())
    .filter((line) => line.length > 0 && !line.startsWith(":"));
  if (lines.length === 0) {
    return null;
  }

  const dataLines: string[] = [];
  let eventType = "message";
  for (const line of lines) {
    if (line.startsWith("event:")) {
      eventType = line.slice("event:".length).trim();
      continue;
    }
    if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trimStart());
    }
  }

  if (eventType !== "message" || dataLines.length === 0) {
    return null;
  }

  const candidate = JSON.parse(dataLines.join("\n")) as Partial<FactoryEvent>;
  if (
    typeof candidate.id !== "string" ||
    typeof candidate.type !== "string" ||
    typeof candidate.context !== "object" ||
    candidate.context === null ||
    typeof candidate.payload !== "object" ||
    candidate.payload === null
  ) {
    throw new Error("invalid factory event replay payload");
  }

  return candidate as FactoryEvent;
}
