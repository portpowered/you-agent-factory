import type { components } from "../generated/openapi";
import type { CanonicalFactoryDefinition } from "../factory-definition";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../session-routing";
import {
  getSessionFactory,
  saveSessionFactory,
  SessionFactoryAPIError,
  type SessionFactoryAPIErrorCode,
  type SessionFactoryDocument,
} from "../session-factory";
import { sessionFactoryAPIErrorMessages } from "../session-factory/messages";
import { currentFactoryDefinitionAPIErrorMessages } from "./messages";

export type { CanonicalFactoryDefinition } from "../factory-definition";

type FactorySaveMode = components["schemas"]["FactorySaveMode"];
export type CurrentFactoryVersion =
  components["schemas"]["HybridLogicalTimestamp"];

/** Dashboard editor and replace-current import activation use session PUT with this mode only. */
export const CURRENT_FACTORY_EDITOR_SAVE_MODE =
  "REPLACE_CURRENT" satisfies FactorySaveMode;
type FactoryValidationTarget = components["schemas"]["FactoryValidationTarget"];

export type CurrentFactoryDocument = SessionFactoryDocument;

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

export interface SaveFactoryForSessionInput {
  baseVersion?: CurrentFactoryVersion;
  factoryDefinition: CanonicalFactoryDefinition;
  includeVersion?: boolean;
  mode: FactorySaveMode;
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
    return await getSessionFactory(resolveSessionID(options.sessionID), {
      fetch: options.fetch,
    });
  } catch (error) {
    throw toCurrentFactoryDefinitionError(error);
  }
}

export async function saveCurrentFactoryDocument(
  input: SaveCurrentFactoryInput,
  options: SaveCurrentFactoryOptions = {},
): Promise<CurrentFactoryDocument> {
  return saveFactoryForSessionDocument(
    {
      baseVersion: input.baseVersion,
      factoryDefinition: input.factoryDefinition,
      mode: CURRENT_FACTORY_EDITOR_SAVE_MODE,
    },
    options,
  );
}

export async function saveFactoryForSessionDocument(
  input: SaveFactoryForSessionInput,
  options: SaveCurrentFactoryOptions = {},
): Promise<CurrentFactoryDocument> {
  try {
    return await saveSessionFactory(
      {
        baseVersion: input.baseVersion,
        factory: input.factoryDefinition,
        includeVersion: input.includeVersion,
        mode: input.mode,
        sessionID: resolveSessionID(options.sessionID),
      },
      {
        fetch: options.fetch,
      },
    );
  } catch (error) {
    throw toCurrentFactoryDefinitionError(error);
  }
}

function resolveSessionID(sessionID: string | null | undefined): string {
  return isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
}

function toCurrentFactoryDefinitionError(error: unknown): CurrentFactoryDefinitionError {
  if (error instanceof CurrentFactoryDefinitionError) {
    return error;
  }

  if (error instanceof SessionFactoryAPIError) {
    return new CurrentFactoryDefinitionError(
      mapSessionFactoryErrorMessage(error.message),
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

  throw error;
}

function mapSessionFactoryErrorCode(
  code: SessionFactoryAPIErrorCode,
): CurrentFactoryDefinitionErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "FACTORY_NOT_IDLE":
    case "INTERNAL_ERROR":
    case "NETWORK_ERROR":
    case "NOT_FOUND":
    case "STALE_FACTORY_VERSION":
      return code;
    case "INVALID_FACTORY":
      return "INVALID_FACTORY_DEFINITION";
    default:
      return "INTERNAL_ERROR";
  }
}

function mapSessionFactoryErrorMessage(message: string): string {
  const sessionFactoryNormalizationPrefix =
    "The session factory API returned a factory definition the dashboard cannot edit.";
  const currentFactoryNormalizationPrefix =
    "The current factory editing API returned a factory definition the dashboard cannot edit.";

  if (message.startsWith(sessionFactoryNormalizationPrefix)) {
    return (
      currentFactoryNormalizationPrefix +
      message.slice(sessionFactoryNormalizationPrefix.length)
    );
  }

  switch (message) {
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
      return message;
  }
}
