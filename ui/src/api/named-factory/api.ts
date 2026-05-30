import type { components } from "../generated/openapi";
import {
  CurrentFactoryDefinitionError,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
  saveFactoryForSessionDocument,
  type CurrentFactoryDocument,
} from "../current-factory-definition";
import { currentFactoryDefinitionAPIErrorMessages } from "../current-factory-definition/messages";
import {
  listFactorySessions,
  openFactorySession,
} from "../factory-sessions";
import {
  extractNamedFactoryNamesFromSessionTargets,
  resolveImportCreateFactoryName,
  type FactoryImportSaveChoice,
} from "./import-save-mode";
import { extractAPIErrorPayload } from "../transport";

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

export async function getCurrentFactory(
  options: GetCurrentFactoryOptions = {},
): Promise<FactoryValue> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new NamedFactoryAPIError("Current factory export is unavailable in this environment.", {
      code: "NETWORK_ERROR",
    });
  }

  try {
    const document = await getCurrentFactoryDocument({
      fetch: fetchImplementation,
      sessionID: options.sessionID,
    });
    return toActivatedFactoryValue(document);
  } catch (error) {
    throw toNamedFactoryAPIErrorForExport(error);
  }
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

const namedFactoryExportAPIErrorMessages = {
  invalidResponse: "The current factory API returned an invalid response.",
  network: "The dashboard could not reach the current factory API.",
  rejectedRequest: "The current factory API rejected the request.",
  unavailableInEnvironment:
    "Current factory export is unavailable in this environment.",
} as const;

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

function toNamedFactoryAPIErrorForExport(error: unknown): NamedFactoryAPIError {
  if (error instanceof CurrentFactoryDefinitionError) {
    return new NamedFactoryAPIError(
      mapCurrentFactoryDefinitionErrorMessageForExport(error),
      {
        code: resolveNamedFactoryAPIErrorCode(error),
        responseBody: error.responseBody,
        status: error.status,
        statusText: error.statusText,
      },
    );
  }

  return toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
}

function mapCurrentFactoryDefinitionErrorMessageForExport(
  error: CurrentFactoryDefinitionError,
): string {
  switch (error.message) {
    case currentFactoryDefinitionAPIErrorMessages.network:
      return namedFactoryExportAPIErrorMessages.network;
    case currentFactoryDefinitionAPIErrorMessages.unavailableInEnvironment:
      return namedFactoryExportAPIErrorMessages.unavailableInEnvironment;
    case currentFactoryDefinitionAPIErrorMessages.invalidResponse:
      return namedFactoryExportAPIErrorMessages.invalidResponse;
    case currentFactoryDefinitionAPIErrorMessages.rejectedRequest:
      return namedFactoryExportAPIErrorMessages.rejectedRequest;
    default:
      return error.message;
  }
}

function resolveNamedFactoryAPIErrorCode(
  error: CurrentFactoryDefinitionError,
): NamedFactoryAPIErrorCode {
  const errorBody = extractAPIErrorPayload(error.responseBody);
  if (typeof errorBody?.code === "string") {
    return normalizeNamedFactoryAPIErrorCode(errorBody.code);
  }

  return normalizeNamedFactoryAPIErrorCodeFromCurrentFactoryDefinition(error.code);
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
