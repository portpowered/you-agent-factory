// biome-ignore-all lint/style/noExcessiveLinesPerFile: typed factory-session transport helpers stay grouped around one API surface.
import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import { withoutEmbeddedSessionDispatches } from "./dispatch-free-session";
import {
  type NormalizedFactorySessionGet,
  normalizeFactorySessionGetResponse,
} from "./normalize-session-get";

export type { NormalizedFactorySessionGet } from "./normalize-session-get";
export type FactorySessionSummary =
  components["schemas"]["FactorySessionSummary"];
export type FactorySession = components["schemas"]["FactorySession"];
export type FactorySessionDurableReadModel =
  components["schemas"]["FactorySessionDurableReadModel"];
export type FactorySessionLiveResult =
  components["schemas"]["FactorySessionLiveResult"];
export type FactorySessionPartialResult =
  components["schemas"]["FactorySessionPartialResult"];
export type FactorySessionArtifactDetail =
  components["schemas"]["FactorySessionArtifactDetail"];
export type FactorySessionTarget =
  components["schemas"]["FactorySessionTarget"];
export type FactorySessionTargetRef =
  components["schemas"]["FactorySessionTargetRef"];
export type FactorySessionsAPIErrorTarget =
  components["schemas"]["FactoryValidationTarget"];
export type OpenFactorySessionResponse =
  components["schemas"]["OpenFactorySessionResponse"];

export type FactorySessionsAPIErrorCode =
  | "BAD_REQUEST"
  | "FACTORY_SESSION_CONFIG_LOAD_FAILED"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export interface FactorySessionsAPIErrorDetails {
  code: FactorySessionsAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
  targets?: FactorySessionsAPIErrorTarget[];
}

export interface ListFactorySessionsOptions {
  fetch?: typeof globalThis.fetch;
}

export interface OpenFactorySessionInput {
  folderPath: string;
  target?: FactorySessionTargetRef;
  validateOnly?: boolean;
  initNewFactory?: boolean;
}

export interface OpenFactorySessionOptions {
  fetch?: typeof globalThis.fetch;
}

export interface CloseFactorySessionOptions {
  fetch?: typeof globalThis.fetch;
}

export interface GetFactorySessionOptions {
  fetch?: typeof globalThis.fetch;
}

export interface GetFactorySessionResultOptions {
  fetch?: typeof globalThis.fetch;
}

export interface GetFactorySessionPartialResultOptions {
  fetch?: typeof globalThis.fetch;
}

export interface GetFactorySessionArtifactOptions {
  fetch?: typeof globalThis.fetch;
}

const FACTORY_SESSIONS_ENDPOINT = "/factory-sessions";

export { FACTORY_SESSIONS_ENDPOINT };

export class FactorySessionsAPIError extends Error {
  public readonly code: FactorySessionsAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;
  public readonly targets?: FactorySessionsAPIErrorTarget[];

  public constructor(
    message: string,
    {
      code,
      responseBody,
      status,
      statusText,
      targets,
    }: FactorySessionsAPIErrorDetails,
  ) {
    super(message);
    this.name = "FactorySessionsAPIError";
    this.code = code;
    this.responseBody = responseBody;
    this.status = status;
    this.statusText = statusText;
    this.targets = targets;
  }
}

export async function listFactorySessions(
  options: ListFactorySessionsOptions = {},
): Promise<FactorySessionSummary[]> {
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
      factoryAPIURL(FACTORY_SESSIONS_ENDPOINT),
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

  if (!isListFactorySessionsResponse(responseBody)) {
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

  return responseBody.sessions.map(withoutEmbeddedSessionDispatches);
}

export async function openFactorySession(
  input: OpenFactorySessionInput,
  options: OpenFactorySessionOptions = {},
): Promise<OpenFactorySessionResponse> {
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
      factoryAPIURL(FACTORY_SESSIONS_ENDPOINT),
      {
        body: JSON.stringify(input),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
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

  if (!isOpenFactorySessionResponse(responseBody)) {
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

  return responseBody.session
    ? {
        ...responseBody,
        session: withoutEmbeddedSessionDispatches(responseBody.session),
      }
    : responseBody;
}

export async function getFactorySession(
  sessionID: string,
  options: GetFactorySessionOptions = {},
): Promise<NormalizedFactorySessionGet> {
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

  try {
    return normalizeFactorySessionGetResponse(responseBody);
  } catch {
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
}

export async function getFactorySessionResult(
  sessionID: string,
  options: GetFactorySessionResultOptions = {},
): Promise<FactorySessionLiveResult> {
  return readFactorySessionResultSurface(sessionID, "result", options.fetch);
}

export async function getFactorySessionPartialResult(
  sessionID: string,
  options: GetFactorySessionPartialResultOptions = {},
): Promise<FactorySessionPartialResult> {
  return readFactorySessionResultSurface(
    sessionID,
    "partial-result",
    options.fetch,
  );
}

export async function getFactorySessionArtifact(
  sessionID: string,
  artifactID: string,
  options: GetFactorySessionArtifactOptions = {},
): Promise<FactorySessionArtifactDetail> {
  const fetchFn = options.fetch ?? globalThis.fetch;
  if (typeof fetchFn !== "function") {
    throw new FactorySessionsAPIError(
      "Factory sessions are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchFn(
      factoryAPIURL(
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}/artifacts/${encodeURIComponent(artifactID)}`,
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

  if (!isFactorySessionArtifactDetail(responseBody)) {
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

export async function closeFactorySession(
  sessionID: string,
  options: CloseFactorySessionOptions = {},
): Promise<void> {
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
        method: "DELETE",
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
}

export function buildFactorySessionsAPIError(
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
    case "FACTORY_SESSION_CONFIG_LOAD_FAILED":
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

async function readFactorySessionResultSurface<
  T extends FactorySessionLiveResult | FactorySessionPartialResult,
>(
  sessionID: string,
  surface: "result" | "partial-result",
  fetchImplementation: typeof globalThis.fetch | undefined,
): Promise<T> {
  const fetchFn = fetchImplementation ?? globalThis.fetch;
  if (typeof fetchFn !== "function") {
    throw new FactorySessionsAPIError(
      "Factory sessions are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchFn(
      factoryAPIURL(
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}/${surface}`,
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

  if (
    !isAPIRecord(responseBody) ||
    typeof responseBody.sessionId !== "string"
  ) {
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

  return responseBody as T;
}

function isFactorySessionArtifactDetail(
  value: unknown,
): value is FactorySessionArtifactDetail {
  return (
    isAPIRecord(value) &&
    typeof value.sessionId === "string" &&
    typeof value.id === "string" &&
    typeof value.kind === "string" &&
    typeof value.visibility === "string"
  );
}
function isListFactorySessionsResponse(
  value: unknown,
): value is { sessions: FactorySessionSummary[] } {
  return isAPIRecord(value) && Array.isArray(value.sessions);
}

function isOpenFactorySessionResponse(
  value: unknown,
): value is OpenFactorySessionResponse {
  return (
    isAPIRecord(value) &&
    (value.session === undefined || isAPIRecord(value.session)) &&
    (value.targets === undefined || Array.isArray(value.targets)) &&
    (value.initsNewFactory === undefined ||
      typeof value.initsNewFactory === "boolean") &&
    (value.folderPath === undefined || typeof value.folderPath === "string")
  );
}
