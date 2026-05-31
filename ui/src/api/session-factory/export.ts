import {
  CurrentFactoryDefinitionError,
  getCurrentFactoryDocument,
  type CurrentFactoryDocument,
} from "../current-factory-definition";
import { extractAPIErrorPayload } from "../transport";
import {
  normalizeSessionFactoryAPIErrorCode,
  SessionFactoryAPIError,
  type SessionFactoryAPIErrorCode,
} from "./errors";
import type { FactoryValue } from "./types";

export type { FactoryValue } from "./types";

export interface GetCurrentFactoryOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export async function getCurrentFactory(
  options: GetCurrentFactoryOptions = {},
): Promise<FactoryValue> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new SessionFactoryAPIError("Current factory export is unavailable in this environment.", {
      code: "NETWORK_ERROR",
    });
  }

  try {
    const document = await getCurrentFactoryDocument({
      fetch: fetchImplementation,
      sessionID: options.sessionID,
    });
    return toExportFactoryValue(document);
  } catch (error) {
    throw toSessionFactoryAPIErrorFromCurrentFactoryDefinition(error);
  }
}

function toExportFactoryValue(document: CurrentFactoryDocument): FactoryValue {
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

  return normalizeSessionFactoryAPIErrorCodeFromCurrentFactoryDefinition(error.code);
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
    return new SessionFactoryAPIError(error.message, { code: "INTERNAL_ERROR" });
  }

  return new SessionFactoryAPIError("Factory activation failed.", { code: "INTERNAL_ERROR" });
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
