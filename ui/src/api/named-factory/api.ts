import type { components } from "../generated/openapi";
import {
  CurrentFactoryDefinitionError,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
  saveFactoryForSessionDocument,
  type CurrentFactoryDocument,
} from "../current-factory-definition";
import {
  listFactorySessions,
  openFactorySession,
} from "../factory-sessions";
import { currentFactorySessionPath } from "../session-routing";
import {
  extractNamedFactoryNamesFromSessionTargets,
  resolveImportCreateFactoryName,
} from "./import-save-mode";
import { factoryAPIURL } from "../baseUrl";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";

export type FactoryValue = components["schemas"]["Factory"];

export type NamedFactoryAPIErrorCode =
  | "BAD_REQUEST"
  | "FACTORY_ALREADY_EXISTS"
  | "FACTORY_NOT_IDLE"
  | "INTERNAL_ERROR"
  | "INVALID_FACTORY"
  | "INVALID_FACTORY_NAME"
  | "NETWORK_ERROR"
  | "NOT_FOUND"
  | "STALE_FACTORY_VERSION";

export interface NamedFactoryAPIErrorDetails {
  code: NamedFactoryAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export interface CreateFactoryOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface GetCurrentFactoryOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface ActivateImportedFactoryForSessionOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface ActivateImportedFactoryAsNewNamedOptions {
  fetch?: typeof globalThis.fetch;
  existingNamedFactoryNames?: readonly string[];
  sessionID?: string | null;
}

export interface DiscoverSessionNamedFactoryNamesOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export type { FactoryImportSaveChoice } from "./import-save-mode";
export {
  allocateFirstFreeSuffixedFactoryName,
  extractNamedFactoryNamesFromSessionTargets,
  resolveImportCreateFactoryName,
} from "./import-save-mode";

export class NamedFactoryAPIError extends Error {
  public readonly code: NamedFactoryAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(message: string, details: NamedFactoryAPIErrorDetails) {
    super(message);
    this.name = "NamedFactoryAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

export async function createFactory(
  value: FactoryValue,
  options: CreateFactoryOptions = {},
): Promise<FactoryValue> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new NamedFactoryAPIError("Named factory activation is unavailable in this environment.", {
      code: "NETWORK_ERROR",
    });
  }

  const { version: _version, ...factoryWithoutVersion } = value;

  let savedDocument: CurrentFactoryDocument;
  try {
    savedDocument = await saveFactoryForSessionDocument(
      {
        factoryDefinition: factoryWithoutVersion,
        includeVersion: false,
        mode: "UPSERT_NAMED_AND_ACTIVATE",
      },
      {
        fetch: fetchImplementation,
        sessionID: options.sessionID,
      },
    );
  } catch (error) {
    throw toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  return toActivatedFactoryValue(savedDocument);
}

export async function getCurrentFactory(
  options: GetCurrentFactoryOptions = {},
): Promise<FactoryValue> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new NamedFactoryAPIError("Current factory export is unavailable in this environment.", {
      code: "NETWORK_ERROR",
    });
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(currentFactorySessionPath(options.sessionID)),
      {
        method: "GET",
      },
    );
  } catch (error) {
    throw new NamedFactoryAPIError("The dashboard could not reach the current factory API.", {
      code: "NETWORK_ERROR",
      responseBody: error,
    });
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorBody = extractAPIErrorPayload(responseBody);
    throw new NamedFactoryAPIError(
      errorBody?.message ?? "The current factory API rejected the request.",
      {
        code: normalizeNamedFactoryAPIErrorCode(errorBody?.code),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isFactoryValue(responseBody)) {
    throw new NamedFactoryAPIError("The current factory API returned an invalid response.", {
      code: "INTERNAL_ERROR",
      responseBody,
      status: response.status,
      statusText: response.statusText,
    });
  }

  return responseBody;
}

export async function activateImportedFactoryForSession(
  importedFactory: FactoryValue,
  options: ActivateImportedFactoryForSessionOptions = {},
): Promise<FactoryValue> {
  let currentDocument: CurrentFactoryDocument;
  try {
    currentDocument = await getCurrentFactoryDocument({
      fetch: options.fetch,
      sessionID: options.sessionID,
    });
  } catch (error) {
    throw toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  let savedDocument: CurrentFactoryDocument;
  try {
    savedDocument = await saveCurrentFactoryDocument(
      {
        baseVersion: currentDocument.version,
        factoryDefinition: toImportedFactoryDefinition(
          importedFactory,
          currentDocument.name,
        ),
      },
      {
        fetch: options.fetch,
        sessionID: options.sessionID,
      },
    );
  } catch (error) {
    throw toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  return toActivatedFactoryValue(savedDocument);
}

export async function discoverSessionNamedFactoryNames(
  options: DiscoverSessionNamedFactoryNamesOptions = {},
): Promise<string[]> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  const sessions = await listFactorySessions({ fetch: fetchImplementation });
  const session = sessions.find((entry) => entry.id === normalizeSessionID(options.sessionID));
  if (!session?.folderPath) {
    return [];
  }

  const response = await openFactorySession(
    {
      folderPath: session.folderPath,
      validateOnly: true,
    },
    { fetch: fetchImplementation },
  );

  return extractNamedFactoryNamesFromSessionTargets(response.targets);
}

export async function activateImportedFactoryAsNewNamedForSession(
  importedFactory: FactoryValue,
  options: ActivateImportedFactoryAsNewNamedOptions = {},
): Promise<FactoryValue> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  const existingNamedFactoryNames =
    options.existingNamedFactoryNames ??
    (await discoverSessionNamedFactoryNames({
      fetch: fetchImplementation,
      sessionID: options.sessionID,
    }));
  const preferredName =
    typeof importedFactory.name === "string" ? importedFactory.name : "";
  const { factoryName, replacesExisting } = resolveImportCreateFactoryName(
    preferredName,
    existingNamedFactoryNames,
  );

  let currentDocument: CurrentFactoryDocument | undefined;
  if (replacesExisting) {
    try {
      currentDocument = await getCurrentFactoryDocument({
        fetch: fetchImplementation,
        sessionID: options.sessionID,
      });
    } catch (error) {
      throw toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
    }
  }

  const { version: _version, ...importedWithoutVersion } = importedFactory;
  const factoryDefinition: FactoryValue = {
    ...importedWithoutVersion,
    name: factoryName,
  };

  let savedDocument: CurrentFactoryDocument;
  try {
    savedDocument = await saveFactoryForSessionDocument(
      {
        baseVersion:
          replacesExisting && currentDocument?.name === factoryName
            ? currentDocument.version
            : undefined,
        factoryDefinition,
        includeVersion: replacesExisting,
        mode: "UPSERT_NAMED_AND_ACTIVATE",
      },
      {
        fetch: fetchImplementation,
        sessionID: options.sessionID,
      },
    );
  } catch (error) {
    throw toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  return toActivatedFactoryValue(savedDocument);
}

function normalizeSessionID(sessionID: string | null | undefined): string {
  const trimmed = sessionID?.trim();
  return trimmed ? trimmed : "~default";
}

function normalizeNamedFactoryAPIErrorCode(code: string | undefined): NamedFactoryAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "FACTORY_ALREADY_EXISTS":
    case "FACTORY_NOT_IDLE":
    case "INTERNAL_ERROR":
    case "INVALID_FACTORY":
    case "INVALID_FACTORY_NAME":
    case "NOT_FOUND":
    case "STALE_FACTORY_VERSION":
      return code;
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}

function isFactoryValue(value: unknown): value is FactoryValue {
  return (
    isAPIRecord(value) &&
    typeof value.name === "string" &&
    value.factory === undefined
  );
}

function toImportedFactoryDefinition(
  importedFactory: FactoryValue,
  sessionFactoryName: string,
): FactoryValue {
  const { version: _version, ...importedWithoutVersion } = importedFactory;
  return {
    ...importedWithoutVersion,
    name: sessionFactoryName,
  };
}

function toActivatedFactoryValue(document: CurrentFactoryDocument): FactoryValue {
  const { version: _version, ...factoryValue } = document;
  return factoryValue;
}

function toNamedFactoryAPIErrorFromCurrentFactoryDefinition(
  error: unknown,
): NamedFactoryAPIError {
  if (error instanceof CurrentFactoryDefinitionError) {
    return new NamedFactoryAPIError(error.message, {
      code: normalizeNamedFactoryAPIErrorCodeFromCurrentFactoryDefinition(error.code),
      responseBody: error.responseBody,
      status: error.status,
      statusText: error.statusText,
    });
  }

  if (error instanceof NamedFactoryAPIError) {
    return error;
  }

  if (error instanceof Error) {
    return new NamedFactoryAPIError(error.message, { code: "INTERNAL_ERROR" });
  }

  return new NamedFactoryAPIError("Factory activation failed.", { code: "INTERNAL_ERROR" });
}

function normalizeNamedFactoryAPIErrorCodeFromCurrentFactoryDefinition(
  code: CurrentFactoryDefinitionError["code"],
): NamedFactoryAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "FACTORY_NOT_IDLE":
    case "NETWORK_ERROR":
    case "NOT_FOUND":
    case "STALE_FACTORY_VERSION":
      return code;
    case "INVALID_FACTORY_DEFINITION":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INVALID_FACTORY";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}
