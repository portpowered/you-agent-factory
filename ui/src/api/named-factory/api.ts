import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
} from "../current-factory-definition";
import { SessionFactoryAPIError } from "../session-factory/errors";
import {
  type ActivateImportedFactoryForSessionOptions,
  activateImportedFactoryForSession as activateImportedFactoryForSessionImpl,
  type DiscoverSessionNamedFactoryNamesOptions,
  discoverSessionNamedFactoryNames as discoverSessionNamedFactoryNamesImpl,
  type FactoryImportSaveChoice,
  type ImportFactoryValue,
} from "../session-factory/import-activation";
import { extractAPIErrorPayload } from "../transport";

export {
  allocateFirstFreeSuffixedFactoryName,
  extractNamedFactoryNamesFromSessionTargets,
  resolveImportCreateFactoryName,
} from "../session-factory/import-save-mode";

export type FactoryValue = ImportFactoryValue;

export type { FactoryImportSaveChoice };

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

export type {
  ActivateImportedFactoryForSessionOptions,
  DiscoverSessionNamedFactoryNamesOptions,
};

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
    throw new NamedFactoryAPIError(
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
    throw toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }
}

export async function discoverSessionNamedFactoryNames(
  options: DiscoverSessionNamedFactoryNamesOptions = {},
): Promise<string[]> {
  try {
    return await discoverSessionNamedFactoryNamesImpl(options);
  } catch (error) {
    throw toNamedFactoryAPIErrorFromSessionFactory(error);
  }
}

export async function activateImportedFactoryForSession(
  importedFactory: FactoryValue,
  options: ActivateImportedFactoryForSessionOptions = {},
): Promise<FactoryValue> {
  try {
    return await activateImportedFactoryForSessionImpl(
      importedFactory,
      options,
    );
  } catch (error) {
    throw toNamedFactoryAPIErrorFromSessionFactory(error);
  }
}

function toActivatedFactoryValue(
  document: CurrentFactoryDocument,
): FactoryValue {
  const { version: _version, ...factoryValue } = document;
  return factoryValue;
}

function normalizeNamedFactoryAPIErrorCode(
  code: string | undefined,
): NamedFactoryAPIErrorCode {
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

function resolveNamedFactoryAPIErrorCode(
  error: CurrentFactoryDefinitionError,
): NamedFactoryAPIErrorCode {
  const errorBody = extractAPIErrorPayload(error.responseBody);
  if (typeof errorBody?.code === "string") {
    return normalizeNamedFactoryAPIErrorCode(errorBody.code);
  }

  return normalizeNamedFactoryAPIErrorCodeFromCurrentFactoryDefinition(
    error.code,
  );
}

function toNamedFactoryAPIErrorFromCurrentFactoryDefinition(
  error: unknown,
): NamedFactoryAPIError {
  if (error instanceof CurrentFactoryDefinitionError) {
    return new NamedFactoryAPIError(error.message, {
      code: resolveNamedFactoryAPIErrorCode(error),
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

  return new NamedFactoryAPIError("Factory activation failed.", {
    code: "INTERNAL_ERROR",
  });
}

function toNamedFactoryAPIErrorFromSessionFactory(
  error: unknown,
): NamedFactoryAPIError {
  if (error instanceof SessionFactoryAPIError) {
    return new NamedFactoryAPIError(error.message, {
      code: error.code,
      responseBody: error.responseBody,
      status: error.status,
      statusText: error.statusText,
    });
  }

  return toNamedFactoryAPIErrorFromCurrentFactoryDefinition(error);
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
