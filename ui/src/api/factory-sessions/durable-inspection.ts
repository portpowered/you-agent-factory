import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import {
  FactorySessionsAPIError,
  type FactorySessionsAPIErrorCode,
  type FactorySessionsAPIErrorTarget,
} from "./api";

export type FactorySessionDispatchSummary =
  components["schemas"]["FactorySessionDispatchSummary"];
export type FactorySessionArtifactSummary =
  components["schemas"]["FactorySessionArtifactSummary"];
type ListFactorySessionDispatchesResponse =
  components["schemas"]["ListFactorySessionDispatchesResponse"];
type ListFactorySessionArtifactsResponse =
  components["schemas"]["ListFactorySessionArtifactsResponse"];

export interface ListDurableFactorySessionDispatchesOptions {
  fetch?: typeof globalThis.fetch;
}

export interface ListDurableFactorySessionArtifactsOptions {
  fetch?: typeof globalThis.fetch;
}

const FACTORY_SESSIONS_ENDPOINT = "/factory-sessions";

export async function listDurableFactorySessionDispatches(
  sessionID: string,
  options: ListDurableFactorySessionDispatchesOptions = {},
): Promise<FactorySessionDispatchSummary[]> {
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
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}/dispatches`,
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

  if (!isListFactorySessionDispatchesResponse(responseBody)) {
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

  return responseBody.dispatches;
}

export async function listDurableFactorySessionArtifacts(
  sessionID: string,
  options: ListDurableFactorySessionArtifactsOptions = {},
): Promise<FactorySessionArtifactSummary[]> {
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
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}/artifacts`,
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

  if (!isListFactorySessionArtifactsResponse(responseBody)) {
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

  return responseBody.artifacts;
}

function buildFactorySessionsAPIError(
  response: Response,
  responseBody: unknown,
  fallbackMessage: string,
): FactorySessionsAPIError {
  const errorBody = extractAPIErrorPayload(responseBody);
  return new FactorySessionsAPIError(errorBody?.message ?? fallbackMessage, {
    code: normalizeFactorySessionsAPIErrorCode(errorBody?.code),
    responseBody,
    status: response.status,
    statusText: response.statusText,
    targets: readFactorySessionsAPIErrorTargets(errorBody?.targets),
  });
}

function normalizeFactorySessionsAPIErrorCode(
  code: string | undefined,
): FactorySessionsAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
      return code;
    case "NOT_FOUND":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}

function readFactorySessionsAPIErrorTargets(
  value: unknown,
): FactorySessionsAPIErrorTarget[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const targets = value.filter(isFactorySessionsAPIErrorTarget);
  return targets.length > 0 ? targets : undefined;
}

function isFactorySessionsAPIErrorTarget(
  value: unknown,
): value is FactorySessionsAPIErrorTarget {
  if (!isAPIRecord(value) || typeof value.code !== "string") {
    return false;
  }
  if (typeof value.message !== "string" || typeof value.severity !== "string") {
    return false;
  }
  if (!isAPIRecord(value.subject)) {
    return false;
  }
  if (
    typeof value.subject.type !== "string" ||
    typeof value.subject.id !== "string" ||
    typeof value.subject.location !== "string"
  ) {
    return false;
  }
  return true;
}

function isListFactorySessionDispatchesResponse(
  value: unknown,
): value is ListFactorySessionDispatchesResponse {
  return (
    isAPIRecord(value) &&
    typeof value.sessionId === "string" &&
    Array.isArray(value.dispatches)
  );
}

function isListFactorySessionArtifactsResponse(
  value: unknown,
): value is ListFactorySessionArtifactsResponse {
  return (
    isAPIRecord(value) &&
    typeof value.sessionId === "string" &&
    Array.isArray(value.artifacts)
  );
}
