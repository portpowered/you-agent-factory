import type { components } from "../generated/openapi";
import { factoryAPIURL } from "../baseUrl";
import {
  CurrentFactoryDefinitionError,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
  type CurrentFactoryDocument,
  type CurrentFactoryVersion,
} from "../current-factory-definition";
import {
  saveSessionFactory,
  SessionFactoryAPIError,
} from "../session-factory";
import {
  DEFAULT_FACTORY_SESSION_ID,
  currentFactorySessionPath,
  isDefaultFactorySessionID,
} from "../session-routing";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";

export type FactoryValue = components["schemas"]["Factory"];

export type FactoryImportSaveChoice = "replace_current" | "create_new_named";

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
}

export interface GetCurrentFactoryOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export interface ActivateImportedFactoryForSessionOptions {
  choice?: FactoryImportSaveChoice;
  createFactoryName?: string;
  existingFactoryNames?: readonly string[];
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

const CREATE_NAMED_FACTORY_ENDPOINT = "/factories";

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

  let response: Response;
  try {
    response = await fetchImplementation(factoryAPIURL(CREATE_NAMED_FACTORY_ENDPOINT), {
      body: JSON.stringify(value),
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    });
  } catch (error) {
    throw new NamedFactoryAPIError("The dashboard could not reach the factory activation API.", {
      code: "NETWORK_ERROR",
      responseBody: error,
    });
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorBody = extractAPIErrorPayload(responseBody);
    throw new NamedFactoryAPIError(
      errorBody?.message ?? "The factory activation API rejected the request.",
      {
        code: normalizeNamedFactoryAPIErrorCode(errorBody?.code),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isFactoryValue(responseBody)) {
    throw new NamedFactoryAPIError("The factory activation API returned an invalid response.", {
      code: "INTERNAL_ERROR",
      responseBody,
      status: response.status,
      statusText: response.statusText,
    });
  }

  return responseBody;
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
  if (options.choice === "create_new_named") {
    return activateImportedFactoryCreateNamedForSession(importedFactory, options);
  }

  return activateImportedFactoryReplaceCurrentForSession(importedFactory, options);
}

async function activateImportedFactoryReplaceCurrentForSession(
  importedFactory: FactoryValue,
  options: ActivateImportedFactoryForSessionOptions,
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

async function activateImportedFactoryCreateNamedForSession(
  importedFactory: FactoryValue,
  options: ActivateImportedFactoryForSessionOptions,
): Promise<FactoryValue> {
  const createFactoryName = options.createFactoryName?.trim();
  if (!createFactoryName) {
    throw new NamedFactoryAPIError("A factory name is required to create a new named factory.", {
      code: "INVALID_FACTORY_NAME",
    });
  }

  const sessionID = resolveActivateImportedFactorySessionID(options.sessionID);
  let currentDocument: CurrentFactoryDocument;
  try {
    currentDocument = await getCurrentFactoryDocument({
      fetch: options.fetch,
      sessionID: options.sessionID,
    });
  } catch (error) {
    throw toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }

  const factoryForSave = toCreateNamedImportedFactoryDefinition(
    importedFactory,
    createFactoryName,
    currentDocument,
    options.existingFactoryNames,
  );

  let savedDocument: CurrentFactoryDocument;
  try {
    savedDocument = await saveSessionFactory(
      {
        factory: factoryForSave,
        mode: "UPSERT_NAMED_AND_ACTIVATE",
        sessionID,
      },
      { fetch: options.fetch },
    );
  } catch (error) {
    throw toNamedFactoryAPIErrorFromSessionFactory(error);
  }

  return toActivatedFactoryValue(savedDocument);
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

function resolveActivateImportedFactorySessionID(
  sessionID: string | null | undefined,
): string {
  return isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
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

function toCreateNamedImportedFactoryDefinition(
  importedFactory: FactoryValue,
  createFactoryName: string,
  currentDocument: CurrentFactoryDocument,
  existingFactoryNames: readonly string[] | undefined,
): FactoryValue {
  const { version: _version, ...importedWithoutVersion } = importedFactory;
  const factory: FactoryValue = {
    ...importedWithoutVersion,
    name: createFactoryName,
  };

  if (!shouldIncludeVersionForImportCreateNamed(createFactoryName, currentDocument, existingFactoryNames)) {
    return factory;
  }

  return {
    ...factory,
    version: incrementImportedFactoryVersion(currentDocument.version),
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
    [
      currentDocument.name,
      ...(existingFactoryNames ?? []),
    ]
      .map((name) => name.trim())
      .filter((name) => name.length > 0),
  );

  return knownExistingNames.has(normalizedCreateFactoryName);
}

function incrementImportedFactoryVersion(
  version: CurrentFactoryVersion,
): CurrentFactoryVersion {
  return {
    logical: (BigInt(version.logical) + 1n).toString(),
    physical: incrementImportedFactoryVersionPhysical(version.physical),
  };
}

function incrementImportedFactoryVersionPhysical(physical: string): string {
  const parsed = Date.parse(physical);
  if (!Number.isFinite(parsed)) {
    return physical;
  }
  return new Date(parsed + 1).toISOString();
}

function toActivatedFactoryValue(document: CurrentFactoryDocument): FactoryValue {
  const { version: _version, ...factoryValue } = document;
  return factoryValue;
}

function toNamedFactoryAPIErrorFromSessionFactory(error: unknown): NamedFactoryAPIError {
  if (error instanceof SessionFactoryAPIError) {
    return new NamedFactoryAPIError(error.message, {
      code: normalizeNamedFactoryAPIErrorCode(error.code),
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
