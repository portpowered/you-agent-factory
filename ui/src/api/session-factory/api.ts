import { factoryAPIURL } from "../baseUrl";
import {
  type CanonicalFactoryDefinition,
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
} from "../factory-definition";
import type { components } from "../generated/openapi";
import { currentFactorySessionPath } from "../session-routing";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import { sessionFactoryAPIErrorMessages } from "./messages";

export type SessionFactorySaveMode = components["schemas"]["FactorySaveMode"];
export type SessionFactory = components["schemas"]["Factory"];
export type SessionFactoryVersion =
  components["schemas"]["HybridLogicalTimestamp"];
type FactoryValidationTarget = components["schemas"]["FactoryValidationTarget"];

export type SessionFactoryDocument = CanonicalFactoryDefinition & {
  version: SessionFactoryVersion;
};

export type SessionFactoryAPIErrorCode =
  | "BAD_REQUEST"
  | "FACTORY_NOT_IDLE"
  | "INTERNAL_ERROR"
  | "INVALID_FACTORY"
  | "INVALID_FACTORY_DEFINITION"
  | "INVALID_FACTORY_NAME"
  | "NETWORK_ERROR"
  | "NOT_FOUND"
  | "STALE_FACTORY_VERSION";

export interface SessionFactoryAPIErrorDetails {
  cause?: unknown;
  code: SessionFactoryAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
  targets?: FactoryValidationTarget[];
}

export interface SessionFactoryRequestOptions {
  fetch?: typeof globalThis.fetch;
}

export interface SaveSessionFactoryInput {
  sessionID: string;
  mode?: SessionFactorySaveMode;
  factory: SessionFactory;
}

interface RequestSessionFactoryDocumentOptions {
  body?: string;
  fetch?: typeof globalThis.fetch;
  headers?: HeadersInit;
  method: "GET" | "PUT";
  rejectedMessage: string;
  sessionID: string;
}

type FactoryDocumentRecord = Record<string, unknown> & {
  name?: unknown;
  version: unknown;
};

export class SessionFactoryAPIError extends Error {
  public readonly cause?: unknown;
  public readonly code: SessionFactoryAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;
  public readonly targets?: FactoryValidationTarget[];

  public constructor(message: string, details: SessionFactoryAPIErrorDetails) {
    super(message);
    this.name = "SessionFactoryAPIError";
    this.cause = details.cause;
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
    this.targets = details.targets;
  }
}

export async function getSessionFactory(
  sessionID: string,
  options: SessionFactoryRequestOptions = {},
): Promise<SessionFactoryDocument> {
  return requestSessionFactoryDocument({
    fetch: options.fetch,
    method: "GET",
    rejectedMessage: sessionFactoryAPIErrorMessages.rejectedRequest,
    sessionID,
  });
}

export async function saveSessionFactory(
  input: SaveSessionFactoryInput,
  options: SessionFactoryRequestOptions = {},
): Promise<SessionFactoryDocument> {
  const requestBody =
    input.mode === undefined
      ? { factory: input.factory }
      : { mode: input.mode, factory: input.factory };

  return requestSessionFactoryDocument({
    body: JSON.stringify(requestBody),
    fetch: options.fetch,
    headers: {
      "content-type": "application/json",
    },
    method: "PUT",
    rejectedMessage: sessionFactoryAPIErrorMessages.rejectedSaveRequest,
    sessionID: input.sessionID,
  });
}

async function requestSessionFactoryDocument({
  body,
  fetch,
  headers,
  method,
  rejectedMessage,
  sessionID,
}: RequestSessionFactoryDocumentOptions): Promise<SessionFactoryDocument> {
  const fetchImplementation = fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new SessionFactoryAPIError(
      sessionFactoryAPIErrorMessages.unavailableInEnvironment,
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(currentFactorySessionPath(sessionID)),
      {
        body,
        headers,
        method,
      },
    );
  } catch (error) {
    throw new SessionFactoryAPIError(sessionFactoryAPIErrorMessages.network, {
      cause: error,
      code: "NETWORK_ERROR",
      responseBody: error,
    });
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorBody = extractAPIErrorPayload(responseBody, {
      isTarget: isErrorTarget,
    });
    throw new SessionFactoryAPIError(errorBody?.message ?? rejectedMessage, {
      code: normalizeSessionFactoryAPIErrorCode(errorBody?.code),
      responseBody,
      status: response.status,
      statusText: response.statusText,
      targets: errorBody?.targets,
    });
  }

  return normalizeSessionFactoryDocument(responseBody, {
    status: response.status,
    statusText: response.statusText,
  });
}

function normalizeSessionFactoryDocument(
  responseBody: unknown,
  responseDetails: Pick<SessionFactoryAPIErrorDetails, "status" | "statusText">,
): SessionFactoryDocument {
  if (!isSessionFactoryDocumentValue(responseBody)) {
    throw new SessionFactoryAPIError(
      sessionFactoryAPIErrorMessages.invalidResponse,
      {
        code: "INTERNAL_ERROR",
        responseBody,
        ...responseDetails,
      },
    );
  }

  try {
    const normalizedFactory = normalizeFactoryDefinition(responseBody);
    const version = normalizeSessionFactoryVersion(normalizedFactory.version);

    return {
      ...normalizedFactory,
      version,
    };
  } catch (error) {
    if (error instanceof FactoryDefinitionAPIError) {
      throw new SessionFactoryAPIError(
        `The session factory API returned a factory definition the dashboard cannot use. ${error.message}`,
        {
          cause: error,
          code: "INVALID_FACTORY_DEFINITION",
          responseBody,
          ...responseDetails,
        },
      );
    }

    throw error;
  }
}

function normalizeSessionFactoryVersion(value: unknown): SessionFactoryVersion {
  const record = isAPIRecord(value) ? value : null;
  if (
    !record ||
    !isSessionFactoryVersionLogicalValue(record.logical) ||
    typeof record.physical !== "string"
  ) {
    throw new SessionFactoryAPIError(
      sessionFactoryAPIErrorMessages.invalidResponse,
      {
        code: "INTERNAL_ERROR",
        responseBody: value,
      },
    );
  }

  return {
    logical: String(record.logical),
    physical: record.physical,
  };
}

function isSessionFactoryVersionLogicalValue(
  value: unknown,
): value is number | string {
  if (typeof value === "string") {
    return /^[0-9]+$/.test(value);
  }
  return typeof value === "number" && Number.isFinite(value);
}

function normalizeSessionFactoryAPIErrorCode(
  code: string | undefined,
): SessionFactoryAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "NOT_FOUND":
    case "FACTORY_NOT_IDLE":
    case "STALE_FACTORY_VERSION":
    case "INVALID_FACTORY":
    case "INVALID_FACTORY_NAME":
      return code;
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}

function isSessionFactoryDocumentValue(
  value: unknown,
): value is FactoryDocumentRecord {
  if (!isAPIRecord(value) || !isAPIRecord(value.version)) {
    return false;
  }
  return value.name !== undefined;
}

function isErrorTarget(value: unknown): value is FactoryValidationTarget {
  return (
    isAPIRecord(value) &&
    typeof value.code === "string" &&
    typeof value.message === "string" &&
    typeof value.severity === "string" &&
    isAPIRecord(value.subject) &&
    typeof value.subject.type === "string" &&
    typeof value.subject.id === "string" &&
    typeof value.subject.location === "string"
  );
}
