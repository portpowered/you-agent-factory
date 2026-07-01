import type { FactoryEventReconnectCursor } from "../events";
import type { components } from "../generated/openapi";
import { factorySessionScopedPath } from "../session-routing";
import { factoryAPIURL } from "../baseUrl";
import { isAPIRecord, readAPIResponseBody } from "../transport";
import {
  buildFactorySessionsAPIError,
  FactorySessionsAPIError,
} from "./api";

export type FactorySessionSyncPreflightResponse =
  components["schemas"]["FactorySessionSyncPreflightResponse"];

export interface FactorySessionLogicalResolveHint {
  backendScopeID: string;
  logicalSessionKeyID: string;
}

export interface GetFactorySessionSyncPreflightOptions {
  fetch?: typeof globalThis.fetch;
  logicalResolve?: FactorySessionLogicalResolveHint;
}

export async function getFactorySessionSyncPreflight(
  sessionID: string,
  reconnectCursor?: FactoryEventReconnectCursor,
  options: GetFactorySessionSyncPreflightOptions = {},
): Promise<FactorySessionSyncPreflightResponse> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new FactorySessionsAPIError(
      "Factory sessions are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  const query = buildSyncPreflightQuery(reconnectCursor, options.logicalResolve);

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(
        `${factorySessionScopedPath("/sync-preflight", sessionID)}${query}`,
      ),
      {
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

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    throw buildFactorySessionsAPIError(
      response,
      responseBody,
      "The factory sessions API rejected the request.",
    );
  }

  if (!isFactorySessionSyncPreflightResponse(responseBody)) {
    throw new FactorySessionsAPIError(
      "The factory sessions API returned an invalid response.",
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

function buildSyncPreflightQuery(
  reconnectCursor?: FactoryEventReconnectCursor,
  logicalResolve?: FactorySessionLogicalResolveHint,
): string {
  const params = new URLSearchParams();
  if (reconnectCursor?.afterEventId) {
    params.set("after_event_id", reconnectCursor.afterEventId);
  }
  if (reconnectCursor?.afterSequence != null) {
    params.set("after_sequence", String(reconnectCursor.afterSequence));
  }
  if (logicalResolve?.backendScopeID.trim()) {
    params.set("backend_scope_id", logicalResolve.backendScopeID.trim());
  }
  if (logicalResolve?.logicalSessionKeyID.trim()) {
    params.set(
      "logical_session_key_id",
      logicalResolve.logicalSessionKeyID.trim(),
    );
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

function isFactorySessionSyncPreflightResponse(
  value: unknown,
): value is FactorySessionSyncPreflightResponse {
  if (
    !isAPIRecord(value) ||
    typeof value.requestedSessionId !== "string" ||
    typeof value.reasonCode !== "string" ||
    typeof value.checkpointReusable !== "boolean" ||
    !isAPIRecord(value.reconnectCursor) ||
    typeof value.reconnectCursor.provided !== "boolean" ||
    typeof value.reconnectCursor.validForStreamGeneration !== "boolean"
  ) {
    return false;
  }

  if (!requiresSyncIdentity(value.reasonCode)) {
    return true;
  }

  return (
    typeof value.backendScopeId === "string" &&
    typeof value.factorySessionId === "string" &&
    typeof value.logicalSessionKeyId === "string" &&
    typeof value.streamGenerationId === "string"
  );
}

function requiresSyncIdentity(reasonCode: string): boolean {
  return (
    reasonCode === "ok" ||
    reasonCode === "cursor_stale" ||
    reasonCode === "logical_session_remap"
  );
}
