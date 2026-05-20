import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";

export type FactorySessionSummary =
  components["schemas"]["FactorySessionSummary"];
export type FactorySessionTarget = components["schemas"]["FactorySessionTarget"];
export type FactorySessionTargetRef =
  components["schemas"]["FactorySessionTargetRef"];
export type OpenFactorySessionResponse =
  components["schemas"]["OpenFactorySessionResponse"];

export type FactorySessionsAPIErrorCode =
  | "BAD_REQUEST"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export interface FactorySessionsAPIErrorDetails {
  code: FactorySessionsAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export interface ListFactorySessionsOptions {
  fetch?: typeof globalThis.fetch;
}

export interface OpenFactorySessionInput {
  folderPath: string;
  target?: FactorySessionTargetRef;
}

export interface OpenFactorySessionOptions {
  fetch?: typeof globalThis.fetch;
}

export interface CloseFactorySessionOptions {
  fetch?: typeof globalThis.fetch;
}

interface APIErrorResponse {
  code?: string;
  message?: string;
}

const FACTORY_SESSIONS_ENDPOINT = "/factory-sessions";

export class FactorySessionsAPIError extends Error {
  public readonly code: FactorySessionsAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    { code, responseBody, status, statusText }: FactorySessionsAPIErrorDetails,
  ) {
    super(message);
    this.name = "FactorySessionsAPIError";
    this.code = code;
    this.responseBody = responseBody;
    this.status = status;
    this.statusText = statusText;
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
    response = await fetchImplementation(factoryAPIURL(FACTORY_SESSIONS_ENDPOINT), {
      method: "GET",
    });
  } catch (error) {
    throw new FactorySessionsAPIError(
      "The dashboard could not reach the factory sessions API.",
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readResponseBody(response);
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

  return responseBody.sessions;
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
    response = await fetchImplementation(factoryAPIURL(FACTORY_SESSIONS_ENDPOINT), {
      body: JSON.stringify(input),
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    });
  } catch (error) {
    throw new FactorySessionsAPIError(
      "The dashboard could not reach the factory sessions API.",
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readResponseBody(response);
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
      factoryAPIURL(`${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}`),
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

  const responseBody = await readResponseBody(response);
  if (!response.ok) {
    throw buildFactorySessionsAPIError(
      response,
      responseBody,
      "The factory sessions API rejected the request.",
    );
  }
}

function buildFactorySessionsAPIError(
  response: Response,
  responseBody: unknown,
  fallbackMessage: string,
): FactorySessionsAPIError {
  const errorBody = asAPIErrorResponse(responseBody);
  return new FactorySessionsAPIError(errorBody?.message ?? fallbackMessage, {
    code: normalizeFactorySessionsAPIErrorCode(errorBody?.code),
    responseBody,
    status: response.status,
    statusText: response.statusText,
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

async function readResponseBody(response: Response): Promise<unknown> {
  const rawBody = await response.text();
  if (rawBody.length === 0) {
    return null;
  }

  try {
    return JSON.parse(rawBody) as unknown;
  } catch {
    return rawBody;
  }
}

function isListFactorySessionsResponse(
  value: unknown,
): value is { sessions: FactorySessionSummary[] } {
  return isRecord(value) && Array.isArray(value.sessions);
}

function isOpenFactorySessionResponse(
  value: unknown,
): value is OpenFactorySessionResponse {
  return (
    isRecord(value) &&
    (value.session === undefined || isRecord(value.session)) &&
    (value.targets === undefined || Array.isArray(value.targets))
  );
}

function asAPIErrorResponse(value: unknown): APIErrorResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  return {
    code: typeof value.code === "string" ? value.code : undefined,
    message: typeof value.message === "string" ? value.message : undefined,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
