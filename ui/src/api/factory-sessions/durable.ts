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

export type FactorySessionDurableReadModel =
  components["schemas"]["FactorySessionDurableReadModel"];
export type FactorySessionResult =
  components["schemas"]["FactorySessionResult"];
export type FactorySessionResultMode =
  components["schemas"]["FactorySessionResultMode"];

export interface GetDurableFactorySessionOptions {
  fetch?: typeof globalThis.fetch;
}

export interface GetDurableFactorySessionResultOptions {
  fetch?: typeof globalThis.fetch;
  includeArtifacts?: boolean;
  mode?: FactorySessionResultMode;
}

const FACTORY_SESSIONS_ENDPOINT = "/factory-sessions";
const DURABLE_FACTORY_SESSION_ID_PREFIX = "dur-sess-";

export function isDurableFactorySessionID(sessionID: string): boolean {
  return sessionID.trim().startsWith(DURABLE_FACTORY_SESSION_ID_PREFIX);
}

export async function getDurableFactorySession(
  sessionID: string,
  options: GetDurableFactorySessionOptions = {},
): Promise<FactorySessionDurableReadModel> {
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
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}`,
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

  if (!isFactorySessionDurableReadModel(responseBody)) {
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

export async function getDurableFactorySessionResult(
  sessionID: string,
  options: GetDurableFactorySessionResultOptions = {},
): Promise<FactorySessionResult> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new FactorySessionsAPIError(
      "Factory sessions are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  const query = new URLSearchParams();
  if (options.mode !== undefined) {
    query.set("mode", options.mode);
  }
  if (options.includeArtifacts !== undefined) {
    query.set("includeArtifacts", String(options.includeArtifacts));
  }
  const querySuffix = query.size > 0 ? `?${query.toString()}` : "";

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}/results${querySuffix}`,
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

  if (!isFactorySessionResult(responseBody)) {
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

function isFactorySessionDurableReadModel(
  value: unknown,
): value is FactorySessionDurableReadModel {
  return (
    isAPIRecord(value) &&
    typeof value.sessionId === "string" &&
    typeof value.status === "string" &&
    typeof value.orchestratorKind === "string" &&
    isAPIRecord(value.resolvedSource) &&
    typeof value.resolvedSource.kind === "string"
  );
}

function isFactorySessionResult(value: unknown): value is FactorySessionResult {
  return (
    isAPIRecord(value) &&
    typeof value.sessionId === "string" &&
    typeof value.resultStatus === "string"
  );
}
