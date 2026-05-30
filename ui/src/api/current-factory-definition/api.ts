import type { components } from "../generated/openapi";
import type { CanonicalFactoryDefinition } from "../factory-definition";
import {
  getSessionFactory,
  saveSessionFactory,
  SessionFactoryAPIError,
} from "../session-factory";
import { sessionFactoryAPIErrorMessages } from "../session-factory/messages";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../session-routing";
import { currentFactoryDefinitionAPIErrorMessages } from "./messages";

export type { CanonicalFactoryDefinition } from "../factory-definition";

type CanonicalFactory = components["schemas"]["Factory"];
export type CurrentFactoryVersion =
  components["schemas"]["HybridLogicalTimestamp"];
type FactoryValidationTarget = components["schemas"]["FactoryValidationTarget"];

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
  targets?: FactoryValidationTarget[];
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

export class CurrentFactoryDefinitionError extends Error {
  public readonly cause?: unknown;
  public readonly code: CurrentFactoryDefinitionErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;
  public readonly targets?: FactoryValidationTarget[];

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
  try {
    return await getSessionFactory(resolveCurrentFactorySessionID(options.sessionID), {
      fetch: options.fetch,
    });
  } catch (error) {
    throw mapSessionFactoryError(error);
  }
}

export async function saveCurrentFactoryDocument(
  input: SaveCurrentFactoryInput,
  options: SaveCurrentFactoryOptions = {},
): Promise<CurrentFactoryDocument> {
  const factory: CanonicalFactory = {
    ...input.factoryDefinition,
    version: incrementCurrentFactoryVersion(input.baseVersion),
  };

  try {
    return await saveSessionFactory(
      {
        factory,
        sessionID: resolveCurrentFactorySessionID(options.sessionID),
      },
      { fetch: options.fetch },
    );
  } catch (error) {
    throw mapSessionFactoryError(error);
  }
}

function resolveCurrentFactorySessionID(
  sessionID: string | null | undefined,
): string {
  return isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
}

function mapSessionFactoryError(error: unknown): CurrentFactoryDefinitionError {
  if (!(error instanceof SessionFactoryAPIError)) {
    throw error;
  }

  throw new CurrentFactoryDefinitionError(
    mapSessionFactoryErrorMessage(error),
    {
      cause: error.cause,
      code: mapSessionFactoryErrorCode(error.code),
      responseBody: error.responseBody,
      status: error.status,
      statusText: error.statusText,
      targets: error.targets,
    },
  );
}

function mapSessionFactoryErrorMessage(error: SessionFactoryAPIError): string {
  switch (error.message) {
    case sessionFactoryAPIErrorMessages.network:
      return currentFactoryDefinitionAPIErrorMessages.network;
    case sessionFactoryAPIErrorMessages.unavailableInEnvironment:
      return currentFactoryDefinitionAPIErrorMessages.unavailableInEnvironment;
    case sessionFactoryAPIErrorMessages.rejectedRequest:
      return currentFactoryDefinitionAPIErrorMessages.rejectedRequest;
    case sessionFactoryAPIErrorMessages.rejectedSaveRequest:
      return currentFactoryDefinitionAPIErrorMessages.rejectedSaveRequest;
    case sessionFactoryAPIErrorMessages.invalidResponse:
      return currentFactoryDefinitionAPIErrorMessages.invalidResponse;
    default:
      if (
        error.code === "INVALID_FACTORY_DEFINITION" &&
        error.message.startsWith(
          "The session factory API returned a factory definition the dashboard cannot use.",
        )
      ) {
        return error.message.replace(
          "The session factory API returned a factory definition the dashboard cannot use.",
          "The current factory editing API returned a factory definition the dashboard cannot edit.",
        );
      }
      return error.message;
  }
}

function mapSessionFactoryErrorCode(
  code: SessionFactoryAPIError["code"],
): CurrentFactoryDefinitionErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "NOT_FOUND":
    case "FACTORY_NOT_IDLE":
    case "STALE_FACTORY_VERSION":
    case "INVALID_FACTORY_DEFINITION":
    case "NETWORK_ERROR":
      return code;
    case "INVALID_FACTORY":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INVALID_FACTORY_DEFINITION";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}

function incrementCurrentFactoryVersion(
  version: CurrentFactoryVersion | undefined,
): CurrentFactoryVersion | undefined {
  if (!version) {
    return undefined;
  }

  return {
    logical: (BigInt(version.logical) + 1n).toString(),
    physical: incrementCurrentFactoryVersionPhysical(version.physical),
  };
}

function incrementCurrentFactoryVersionPhysical(physical: string): string {
  const parsed = Date.parse(physical);
  if (!Number.isFinite(parsed)) {
    return physical;
  }
  return new Date(parsed + 1).toISOString();
}
