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
import {
  normalizeSessionFactoryAPIErrorCode,
  SessionFactoryAPIError,
  type SessionFactoryAPIErrorDetails,
} from "./errors";
import { sessionFactoryAPIErrorMessages } from "./messages";
import { resolveSessionFactoryAPIErrorMessage } from "./operator-errors";

export type { CanonicalFactoryDefinition } from "../factory-definition";

type CanonicalFactory = components["schemas"]["Factory"];
type FactorySaveMode = components["schemas"]["FactorySaveMode"];
export type SessionFactoryVersion =
  components["schemas"]["HybridLogicalTimestamp"];

export type SessionFactoryDocument = CanonicalFactoryDefinition & {
  version: SessionFactoryVersion;
};

export interface GetSessionFactoryOptions {
  fetch?: typeof globalThis.fetch;
}

export interface SaveSessionFactoryParams {
  sessionID: string;
  mode?: FactorySaveMode;
  factory: CanonicalFactoryDefinition;
  baseVersion?: SessionFactoryVersion;
  includeVersion?: boolean;
}

export interface SaveSessionFactoryOptions {
  fetch?: typeof globalThis.fetch;
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

export async function getSessionFactory(
  sessionID: string,
  options: GetSessionFactoryOptions = {},
): Promise<SessionFactoryDocument> {
  return requestSessionFactoryDocument({
    fetch: options.fetch,
    method: "GET",
    rejectedMessage: sessionFactoryAPIErrorMessages.rejectedRequest,
    sessionID,
  });
}

export async function saveSessionFactory(
  params: SaveSessionFactoryParams,
  options: SaveSessionFactoryOptions = {},
): Promise<SessionFactoryDocument> {
  const resolvedMode = params.mode ?? "REPLACE_CURRENT";
  const includeVersion =
    params.includeVersion ?? resolvedMode === "REPLACE_CURRENT";
  const factoryPayload: CanonicalFactory = {
    ...params.factory,
  };
  if (includeVersion) {
    factoryPayload.version = incrementSessionFactoryVersion(params.baseVersion);
  } else {
    delete factoryPayload.version;
  }

  const requestBody =
    params.mode !== undefined
      ? {
          mode: params.mode,
          factory: factoryPayload,
        }
      : {
          factory: factoryPayload,
        };

  return requestSessionFactoryDocument({
    body: JSON.stringify(requestBody),
    fetch: options.fetch,
    headers: {
      "content-type": "application/json",
    },
    method: "PUT",
    rejectedMessage: sessionFactoryAPIErrorMessages.rejectedSaveRequest,
    sessionID: params.sessionID,
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
    const code = normalizeSessionFactoryAPIErrorCode(errorBody?.code);
    throw new SessionFactoryAPIError(
      resolveSessionFactoryAPIErrorMessage({
        apiMessage: errorBody?.message,
        code,
        rejectedMessage,
        status: response.status,
      }),
      {
        code,
        responseBody,
        status: response.status,
        statusText: response.statusText,
        targets: errorBody?.targets,
      },
    );
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
  if (!isEditableFactoryDefinitionValue(responseBody)) {
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
        `The session factory API returned a factory definition the dashboard cannot edit. ${error.message}`,
        {
          cause: error,
          code: "INVALID_FACTORY",
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

function incrementSessionFactoryVersion(
  version: SessionFactoryVersion | undefined,
): SessionFactoryVersion | undefined {
  if (!version) {
    return undefined;
  }

  return {
    logical: (BigInt(version.logical) + 1n).toString(),
    physical: incrementSessionFactoryVersionPhysical(version.physical),
  };
}

function incrementSessionFactoryVersionPhysical(physical: string): string {
  const parsed = Date.parse(physical);
  if (!Number.isFinite(parsed)) {
    return physical;
  }
  return new Date(parsed + 1).toISOString();
}

function isSessionFactoryVersionLogicalValue(
  value: unknown,
): value is number | string {
  if (typeof value === "string") {
    return /^[0-9]+$/.test(value);
  }
  return typeof value === "number" && Number.isFinite(value);
}

function isEditableFactoryDefinitionValue(
  value: unknown,
): value is FactoryDocumentRecord {
  if (!isAPIRecord(value) || !isAPIRecord(value.version)) {
    return false;
  }
  return value.name !== undefined;
}

function isErrorTarget(
  value: unknown,
): value is components["schemas"]["FactoryValidationTarget"] {
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
