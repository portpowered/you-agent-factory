import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import { factorySessionScopedPath } from "../session-routing";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";

export type SessionFactoryInvocationRequest =
  components["schemas"]["InvocationRequest"];
export type SessionFactoryInvocationResponse =
  components["schemas"]["InvocationResponse"];
export type SessionFactoryInvocationErrorTarget =
  components["schemas"]["FactoryValidationTarget"];
export type SessionFactoryInvocationErrorCode =
  | components["schemas"]["ErrorResponse"]["code"]
  | "NETWORK_ERROR"
  | string;

export interface SessionFactoryInvocationErrorDetails {
  cause?: unknown;
  code: SessionFactoryInvocationErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
  targets?: SessionFactoryInvocationErrorTarget[];
}

export interface InvokeSessionFactoryOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export class SessionFactoryInvocationError extends Error {
  public readonly cause?: unknown;
  public readonly code: SessionFactoryInvocationErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;
  public readonly targets?: SessionFactoryInvocationErrorTarget[];

  public constructor(
    message: string,
    details: SessionFactoryInvocationErrorDetails,
  ) {
    super(message);
    this.name = "SessionFactoryInvocationError";
    this.cause = details.cause;
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
    this.targets = details.targets;
  }
}

export async function invokeSessionFactory(
  request: SessionFactoryInvocationRequest,
  options: InvokeSessionFactoryOptions = {},
): Promise<SessionFactoryInvocationResponse> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new SessionFactoryInvocationError(
      "Factory invocation is unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(
        factorySessionScopedPath("/invocations", options.sessionID),
      ),
      {
        body: JSON.stringify(request),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      },
    );
  } catch (error) {
    throw new SessionFactoryInvocationError(
      "The dashboard could not reach the session invocation API.",
      {
        cause: error,
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorBody = extractAPIErrorPayload(responseBody);
    throw new SessionFactoryInvocationError(
      errorBody?.message ?? "The session invocation API rejected the request.",
      {
        code: normalizeInvocationErrorCode(errorBody?.code),
        responseBody,
        status: response.status,
        statusText: response.statusText,
        targets: Array.isArray(errorBody?.targets)
          ? (errorBody.targets as SessionFactoryInvocationErrorTarget[])
          : undefined,
      },
    );
  }

  if (!isSessionFactoryInvocationResponse(responseBody)) {
    throw new SessionFactoryInvocationError(
      "The session invocation API returned an invalid response.",
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

function isSessionFactoryInvocationResponse(
  value: unknown,
): value is SessionFactoryInvocationResponse {
  if (!isAPIRecord(value)) {
    return false;
  }

  return (
    typeof value.requestId === "string" &&
    typeof value.traceId === "string" &&
    typeof value.status === "string"
  );
}

function normalizeInvocationErrorCode(
  code: string | undefined,
): SessionFactoryInvocationErrorCode {
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return code && code.length > 0 ? code : "INTERNAL_ERROR";
}
