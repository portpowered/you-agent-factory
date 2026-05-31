import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
  saveFactoryForSessionDocument,
} from "../current-factory-definition";
import { listFactorySessions, openFactorySession } from "../factory-sessions";
import type { components } from "../generated/openapi";
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
  const session = sessions.find(
    (entry) => entry.id === normalizeSessionID(options.sessionID),
  );
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
): Promise<ImportFactoryValue> {
  let currentDocument: CurrentFactoryDocument;
  try {
    currentDocument = await getCurrentFactoryDocument({
      fetch: options.fetch,
      sessionID: options.sessionID,
    });
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
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
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  return toActivatedFactoryValue(savedDocument);
}

async function activateImportedFactoryCreateNamedForSession(
  importedFactory: ImportFactoryValue,
  options: ActivateImportedFactoryForSessionOptions,
): Promise<ImportFactoryValue> {
  const createFactoryName = options.createFactoryName?.trim();
  if (!createFactoryName) {
    throw new SessionFactoryAPIError(
      "A factory name is required to create a new named factory.",
      {
        code: "INVALID_FACTORY_NAME",
      },
    );
  }

  let currentDocument: CurrentFactoryDocument;
  try {
    currentDocument = await getCurrentFactoryDocument({
      fetch: options.fetch,
      sessionID: options.sessionID,
    });
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

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
        factoryDefinition: {
          ...importedWithoutVersion,
          name: createFactoryName,
        },
        includeVersion,
        mode: "UPSERT_NAMED_AND_ACTIVATE",
      },
      {
        fetch: options.fetch,
        sessionID: options.sessionID,
      },
    );
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  return toActivatedFactoryValue(savedDocument);
}

function normalizeSessionID(sessionID: string | null | undefined): string {
  const trimmed = sessionID?.trim();
  return trimmed ? trimmed : "~default";
}

function toImportedFactoryDefinition(
  importedFactory: ImportFactoryValue,
  sessionFactoryName: string,
): ImportFactoryValue {
  const { version: _version, ...importedWithoutVersion } = importedFactory;
  return {
    ...importedWithoutVersion,
    name: sessionFactoryName,
  };
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
