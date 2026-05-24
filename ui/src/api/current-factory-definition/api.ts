import type { components } from "../generated/openapi";
import { factoryAPIURL } from "../baseUrl";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
  type CanonicalFactoryDefinition,
} from "../factory-definition";
import { currentFactorySessionPath } from "../session-routing";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import { currentFactoryDefinitionAPIErrorMessages } from "./messages";

export type { CanonicalFactoryDefinition } from "../factory-definition";

type CanonicalFactory = components["schemas"]["Factory"];
export type CurrentFactoryVersion =
  components["schemas"]["HybridLogicalTimestamp"];
type ErrorTarget = components["schemas"]["ErrorTarget"];

export type CurrentFactoryDocument = CanonicalFactoryDefinition & {
  version: CurrentFactoryVersion;
};

export type CurrentFactoryDefinitionErrorCode =
  | "BAD_REQUEST"
  | "FACTORY_NOT_IDLE"
  | "INTERNAL_ERROR"
  | "INVALID_FACTORY_DEFINITION"
  | "NETWORK_ERROR"
  | "NOT_FOUND"
  | "STALE_FACTORY_VERSION";

export interface CurrentFactoryDefinitionErrorDetails {
  cause?: unknown;
  code: CurrentFactoryDefinitionErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
  targets?: ErrorTarget[];
}

export interface GetCurrentFactoryDefinitionOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface SaveCurrentFactoryOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface SaveCurrentFactoryInput {
  baseVersion?: CurrentFactoryVersion;
  factoryDefinition: CanonicalFactoryDefinition;
}

interface RequestCurrentFactoryDocumentOptions {
  body?: string;
  fetch?: typeof globalThis.fetch;
  headers?: HeadersInit;
  method: "GET" | "PUT";
  rejectedMessage: string;
  sessionID?: string | null;
}

type FactoryDocumentRecord = Record<string, unknown> & {
  name?: unknown;
  version: unknown;
};

export class CurrentFactoryDefinitionError extends Error {
  public readonly cause?: unknown;
  public readonly code: CurrentFactoryDefinitionErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;
  public readonly targets?: ErrorTarget[];

  public constructor(
    message: string,
    details: CurrentFactoryDefinitionErrorDetails,
  ) {
    super(message);
    this.name = "CurrentFactoryDefinitionError";
    this.cause = details.cause;
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
    this.targets = details.targets;
  }
}

export async function getCurrentFactoryDefinition(
  options: GetCurrentFactoryDefinitionOptions = {},
): Promise<CanonicalFactoryDefinition> {
  return getCurrentFactoryDocument(options);
}

export async function getCurrentFactoryDocument(
  options: GetCurrentFactoryDefinitionOptions = {},
): Promise<CurrentFactoryDocument> {
  return requestCurrentFactoryDocument({
    fetch: options.fetch,
    method: "GET",
    rejectedMessage:
      currentFactoryDefinitionAPIErrorMessages.rejectedRequest,
    sessionID: options.sessionID,
  });
}

export async function saveCurrentFactoryDocument(
  input: SaveCurrentFactoryInput,
  options: SaveCurrentFactoryOptions = {},
): Promise<CurrentFactoryDocument> {
  const requestBody: CanonicalFactory = {
    ...input.factoryDefinition,
    version: input.baseVersion,
  };

  return requestCurrentFactoryDocument({
    body: JSON.stringify(requestBody),
    fetch: options.fetch,
    headers: {
      "content-type": "application/json",
    },
    method: "PUT",
    rejectedMessage:
      currentFactoryDefinitionAPIErrorMessages.rejectedSaveRequest,
    sessionID: options.sessionID,
  });
}

async function requestCurrentFactoryDocument({
  body,
  fetch,
  headers,
  method,
  rejectedMessage,
  sessionID,
}: RequestCurrentFactoryDocumentOptions): Promise<CurrentFactoryDocument> {
  const fetchImplementation = fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new CurrentFactoryDefinitionError(
      currentFactoryDefinitionAPIErrorMessages.unavailableInEnvironment,
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
    throw new CurrentFactoryDefinitionError(
      currentFactoryDefinitionAPIErrorMessages.network,
      {
        cause: error,
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorBody = extractAPIErrorPayload(responseBody, {
      isTarget: isErrorTarget,
    });
    throw new CurrentFactoryDefinitionError(
      errorBody?.message ?? rejectedMessage,
      {
        code: normalizeCurrentFactoryDefinitionErrorCode(
          errorBody?.code,
        ),
        responseBody,
        status: response.status,
        statusText: response.statusText,
        targets: errorBody?.targets,
      },
    );
  }

  return normalizeCurrentFactoryDocument(responseBody, {
    status: response.status,
    statusText: response.statusText,
  });
}

function normalizeCurrentFactoryDocument(
  responseBody: unknown,
  responseDetails: Pick<
    CurrentFactoryDefinitionErrorDetails,
    "status" | "statusText"
  >,
): CurrentFactoryDocument {
  if (!isEditableFactoryDefinitionValue(responseBody)) {
    throw new CurrentFactoryDefinitionError(
      currentFactoryDefinitionAPIErrorMessages.invalidResponse,
      {
        code: "INTERNAL_ERROR",
        responseBody,
        ...responseDetails,
      },
    );
  }

  try {
    const normalizedFactory = normalizeFactoryDefinition(responseBody);
    const version = normalizeCurrentFactoryVersion(
      normalizedFactory.version,
    );

    return {
      ...normalizedFactory,
      version,
    };
  } catch (error) {
    if (error instanceof FactoryDefinitionAPIError) {
      throw new CurrentFactoryDefinitionError(
        `The current factory editing API returned a factory definition the dashboard cannot edit. ${error.message}`,
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

function normalizeCurrentFactoryVersion(
  value: unknown,
): CurrentFactoryVersion {
  const record = isAPIRecord(value) ? value : null;
  if (
    !record ||
    typeof record.logical !== "number" ||
    !Number.isFinite(record.logical) ||
    typeof record.physical !== "string"
  ) {
    throw new CurrentFactoryDefinitionError(
      currentFactoryDefinitionAPIErrorMessages.invalidResponse,
      {
        code: "INTERNAL_ERROR",
        responseBody: value,
      },
    );
  }

  return {
    logical: record.logical,
    physical: record.physical,
  };
}

function normalizeCurrentFactoryDefinitionErrorCode(
  code: string | undefined,
): CurrentFactoryDefinitionErrorCode {
  switch (code) {
    case "BAD_REQUEST":
      return code;
    case "NOT_FOUND":
      return code;
    case "FACTORY_NOT_IDLE":
      return code;
    case "STALE_FACTORY_VERSION":
      return code;
    case "INVALID_FACTORY":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INVALID_FACTORY_DEFINITION";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}

function isEditableFactoryDefinitionValue(
  value: unknown,
): value is FactoryDocumentRecord {
  if (!isAPIRecord(value) || !isAPIRecord(value.version)) {
    return false;
  }
  return value.name !== undefined;
}

function isErrorTarget(value: unknown): value is ErrorTarget {
  return isAPIRecord(value) && typeof value.kind === "string";
}
