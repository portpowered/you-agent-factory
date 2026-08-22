import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
  IMPORT_CREATE_NAMED_SAVE_MODE,
  saveFactoryForSessionDocument,
} from "../current-factory-definition";
import { listFactorySessions, openFactorySession } from "../factory-sessions";
import type { components } from "../generated/openapi";
import { isDefaultFactorySessionID } from "../session-routing";
import { extractAPIErrorPayload } from "../transport";
import {
  normalizeSessionFactoryAPIErrorCode,
  SessionFactoryAPIError,
  type SessionFactoryAPIErrorCode,
} from "./errors";
import { extractNamedFactoryNamesFromSessionTargets } from "./import-save-mode";

export {
  allocateFirstFreeSuffixedFactoryName,
  extractNamedFactoryNamesFromSessionTargets,
  resolveImportCreateFactoryName,
} from "./import-save-mode";

export type ImportFactoryValue = components["schemas"]["Factory"];

export type FactoryImportSaveChoice = "replace_current" | "create_new_named";

export interface ActivateImportedFactoryForSessionOptions {
  choice?: FactoryImportSaveChoice;
  createFactoryName?: string;
  currentDocument?: CurrentFactoryDocument | null;
  existingFactoryNames?: readonly string[];
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface DiscoverSessionNamedFactoryNamesOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface GetCurrentFactoryOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export async function getCurrentFactory(
  options: GetCurrentFactoryOptions = {},
): Promise<ImportFactoryValue> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new SessionFactoryAPIError(
      "Current factory export is unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  try {
    const document = await getCurrentFactoryDocument({
      fetch: fetchImplementation,
      sessionID: options.sessionID,
    });
    return toActivatedFactoryValue(document);
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }
}

export async function discoverSessionNamedFactoryNames(
  options: DiscoverSessionNamedFactoryNamesOptions = {},
): Promise<string[]> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  const sessions = await listFactorySessions({ fetch: fetchImplementation });
  const requestedSessionID = normalizeSessionID(options.sessionID);
  const session =
    sessions.find((entry) => entry.id === requestedSessionID) ??
    (isDefaultFactorySessionID(options.sessionID)
      ? sessions.find((entry) => entry.isDefault)
      : undefined);
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

export async function activateImportedFactoryForSession(
  importedFactory: ImportFactoryValue,
  options: ActivateImportedFactoryForSessionOptions = {},
): Promise<ImportFactoryValue> {
  const savedDocument = await activateImportedFactoryDocumentForSession(
    importedFactory,
    options,
  );
  return toActivatedFactoryValue(savedDocument);
}

export async function activateImportedFactoryDocumentForSession(
  importedFactory: ImportFactoryValue,
  options: ActivateImportedFactoryForSessionOptions = {},
): Promise<CurrentFactoryDocument> {
  if (options.choice === "create_new_named") {
    return activateImportedFactoryCreateNamedForSession(
      importedFactory,
      options,
    );
  }

  return activateImportedFactoryReplaceCurrentForSession(
    importedFactory,
    options,
  );
}

async function activateImportedFactoryReplaceCurrentForSession(
  importedFactory: ImportFactoryValue,
  options: ActivateImportedFactoryForSessionOptions,
): Promise<CurrentFactoryDocument> {
  const currentDocument = await resolveActivationCurrentDocument(options);

  const { version: _version, ...importedWithoutVersion } = importedFactory;

  let savedDocument: CurrentFactoryDocument;
  try {
    savedDocument = await saveFactoryForSessionDocument(
      {
        baseVersion: currentDocument.version,
        canonicalFactoryName: currentDocument.name,
        factoryDefinition: importedWithoutVersion,
        mode: "REPLACE_CURRENT",
      },
      {
        fetch: options.fetch,
        sessionID: options.sessionID,
      },
    );
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  return savedDocument;
}

async function activateImportedFactoryCreateNamedForSession(
  importedFactory: ImportFactoryValue,
  options: ActivateImportedFactoryForSessionOptions,
): Promise<CurrentFactoryDocument> {
  const createFactoryName = options.createFactoryName?.trim();
  if (!createFactoryName) {
    throw new SessionFactoryAPIError(
      "A factory name is required to create a new named factory.",
      {
        code: "INVALID_FACTORY_NAME",
      },
    );
  }

  const currentDocument = await resolveActivationCurrentDocument(options);

  const { version: _version, ...importedWithoutVersion } = importedFactory;
  const includeVersion = shouldIncludeVersionForImportCreateNamed(
    createFactoryName,
    currentDocument,
    options.existingFactoryNames,
  );

  let savedDocument: CurrentFactoryDocument;
  try {
    savedDocument = await saveFactoryForSessionDocument(
      {
        baseVersion: includeVersion ? currentDocument.version : undefined,
        canonicalFactoryName: createFactoryName,
        factoryDefinition: importedWithoutVersion,
        includeVersion,
        mode: IMPORT_CREATE_NAMED_SAVE_MODE,
      },
      {
        fetch: options.fetch,
        sessionID: options.sessionID,
      },
    );
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  return savedDocument;
}

function normalizeSessionID(sessionID: string | null | undefined): string {
  const trimmed = sessionID?.trim();
  return trimmed ? trimmed : "~default";
}

async function resolveActivationCurrentDocument(
  options: ActivateImportedFactoryForSessionOptions,
): Promise<CurrentFactoryDocument> {
  if (options.currentDocument) {
    return options.currentDocument;
  }

  try {
    return await getCurrentFactoryDocument({
      fetch: options.fetch,
      sessionID: options.sessionID,
    });
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }
}

function shouldIncludeVersionForImportCreateNamed(
  createFactoryName: string,
  currentDocument: CurrentFactoryDocument,
  existingFactoryNames: readonly string[] | undefined,
): boolean {
  const normalizedCreateFactoryName = createFactoryName.trim();
  if (normalizedCreateFactoryName.length === 0) {
    return false;
  }

  if (normalizedCreateFactoryName === currentDocument.name.trim()) {
    return true;
  }

  const knownExistingNames = new Set(
    [currentDocument.name, ...(existingFactoryNames ?? [])]
      .map((name) => name.trim())
      .filter((name) => name.length > 0),
  );

  return knownExistingNames.has(normalizedCreateFactoryName);
}

function toActivatedFactoryValue(
  document: CurrentFactoryDocument,
): ImportFactoryValue {
  const { version: _version, ...factoryValue } = document;
  return factoryValue;
}

function resolveSessionFactoryAPIErrorCode(
  error: CurrentFactoryDefinitionError,
): SessionFactoryAPIErrorCode {
  const errorBody = extractAPIErrorPayload(error.responseBody);
  if (typeof errorBody?.code === "string") {
    return normalizeSessionFactoryAPIErrorCode(errorBody.code);
  }

  return normalizeSessionFactoryAPIErrorCodeFromCurrentFactoryDefinition(
    error.code,
  );
}

function toSessionFactoryAPIErrorFromCurrentFactoryDefinition(
  error: unknown,
): SessionFactoryAPIError {
  if (error instanceof CurrentFactoryDefinitionError) {
    return new SessionFactoryAPIError(error.message, {
      code: resolveSessionFactoryAPIErrorCode(error),
      responseBody: error.responseBody,
      status: error.status,
      statusText: error.statusText,
    });
  }

  if (error instanceof SessionFactoryAPIError) {
    return error;
  }

  if (error instanceof Error) {
    return new SessionFactoryAPIError(error.message, {
      code: "INTERNAL_ERROR",
    });
  }

  return new SessionFactoryAPIError("Factory activation failed.", {
    code: "INTERNAL_ERROR",
  });
}

function normalizeSessionFactoryAPIErrorCodeFromCurrentFactoryDefinition(
  code: CurrentFactoryDefinitionError["code"],
): SessionFactoryAPIErrorCode {
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
