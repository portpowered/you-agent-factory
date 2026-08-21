import {
  type FactoryReplayTextIssue,
  FactoryReplayTextParseError,
  safeParseFactoryEventReplayText,
} from "@you-agent-factory/client";

import { factoryAPIURL } from "../baseUrl";
import { FACTORY_EVENTS_ENDPOINT, type FactoryEvent } from "../events";
import { factorySessionScopedPath } from "../session-routing";
import { readAPIResponseBody } from "../transport";
import { buildFactorySessionsAPIError, FactorySessionsAPIError } from "./api";

export interface ListFactorySessionEventReplayOptions {
  fetch?: typeof globalThis.fetch;
  onDiagnostics?: (
    diagnostics: readonly FactoryEventReplayDiagnostic[],
  ) => void;
}

export type FactoryEventReplayDiagnostic = FactoryReplayTextIssue;

export type SafeParseFactoryEventReplayStreamResult =
  | {
      data: FactoryEvent[];
      diagnostics: readonly FactoryEventReplayDiagnostic[];
      success: true;
    }
  | {
      diagnostics: readonly FactoryEventReplayDiagnostic[];
      issues: readonly FactoryEventReplayDiagnostic[];
      success: false;
    };

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
  const parsed = safeParseFactoryEventReplayStream(replayStream);
  options.onDiagnostics?.(parsed.diagnostics);
  if (!parsed.success) {
    throw new FactorySessionsAPIError(
      "The factory sessions API returned an invalid response.",
      {
        code: "INTERNAL_ERROR",
        responseBody: new FactoryReplayTextParseError(parsed.issues),
        status: response.status,
        statusText: response.statusText,
      },
    );
  }
  return parsed.data;
}

export function parseFactoryEventReplayStream(
  replayStream: string,
): FactoryEvent[] {
  const result = safeParseFactoryEventReplayStream(replayStream);
  if (!result.success) {
    throw new FactoryReplayTextParseError(result.issues);
  }
  return result.data;
}

export function safeParseFactoryEventReplayStream(
  replayStream: string,
): SafeParseFactoryEventReplayStreamResult {
  const result = safeParseFactoryEventReplayText(replayStream);
  if (!result.success) {
    return {
      diagnostics: result.diagnostics,
      issues: result.issues,
      success: false,
    };
  }
  return {
    data: result.data as unknown as FactoryEvent[],
    diagnostics: result.diagnostics,
    success: true,
  };
}
