import type { components } from "../generated/openapi";
import { factoryAPIURL } from "../baseUrl";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
  type CanonicalFactoryDefinition,
} from "../factory-definition";

export type { CanonicalFactoryDefinition } from "../factory-definition";

type EditableFactoryDefinition =
  components["schemas"]["EditableFactoryDefinition"];
export type EditableFactoryDefinitionVersion =
  components["schemas"]["HybridLogicalTimestamp"];

export interface EditableFactoryDefinitionDocument {
  factoryDefinition: CanonicalFactoryDefinition;
  version: EditableFactoryDefinitionVersion;
}

export type CurrentEditableFactoryDefinitionErrorCode =
  | "INTERNAL_ERROR"
  | "INVALID_FACTORY_DEFINITION"
  | "NETWORK_ERROR"
  | "NOT_FOUND";

export interface CurrentEditableFactoryDefinitionErrorDetails {
  cause?: unknown;
  code: CurrentEditableFactoryDefinitionErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export interface GetCurrentEditableFactoryDefinitionOptions {
  fetch?: typeof globalThis.fetch;
}

interface APIErrorResponse {
  code?: string;
  message?: string;
}

const GET_CURRENT_EDITABLE_FACTORY_DEFINITION_ENDPOINT =
  "/factory/~current/editable-definition";

export class CurrentEditableFactoryDefinitionError extends Error {
  public readonly cause?: unknown;
  public readonly code: CurrentEditableFactoryDefinitionErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: CurrentEditableFactoryDefinitionErrorDetails,
  ) {
    super(message);
    this.name = "CurrentEditableFactoryDefinitionError";
    this.cause = details.cause;
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

export async function getCurrentEditableFactoryDefinition(
  options: GetCurrentEditableFactoryDefinitionOptions = {},
): Promise<CanonicalFactoryDefinition> {
  const document = await getCurrentEditableFactoryDefinitionDocument(options);
  return document.factoryDefinition;
}

export async function getCurrentEditableFactoryDefinitionDocument(
  options: GetCurrentEditableFactoryDefinitionOptions = {},
): Promise<EditableFactoryDefinitionDocument> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new CurrentEditableFactoryDefinitionError(
      "Current factory editing is unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(GET_CURRENT_EDITABLE_FACTORY_DEFINITION_ENDPOINT),
      {
        method: "GET",
      },
    );
  } catch (error) {
    throw new CurrentEditableFactoryDefinitionError(
      "The dashboard could not reach the current factory editing API.",
      {
        cause: error,
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readResponseBody(response);
  if (!response.ok) {
    const errorBody = asAPIErrorResponse(responseBody);
    throw new CurrentEditableFactoryDefinitionError(
      errorBody?.message ??
        "The current factory editing API rejected the request.",
      {
        code: normalizeCurrentEditableFactoryDefinitionErrorCode(
          errorBody?.code,
        ),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  return normalizeEditableFactoryDefinitionDocument(responseBody, {
    status: response.status,
    statusText: response.statusText,
  });
}

function normalizeEditableFactoryDefinitionDocument(
  responseBody: unknown,
  responseDetails: Pick<
    CurrentEditableFactoryDefinitionErrorDetails,
    "status" | "statusText"
  >,
): EditableFactoryDefinitionDocument {
  if (!isEditableFactoryDefinitionValue(responseBody)) {
    throw new CurrentEditableFactoryDefinitionError(
      "The current factory editing API returned an invalid response.",
      {
        code: "INTERNAL_ERROR",
        responseBody,
        ...responseDetails,
      },
    );
  }

  try {
    return {
      factoryDefinition: normalizeFactoryDefinition(
        responseBody.factoryDefinition,
      ),
      version: normalizeEditableFactoryDefinitionVersion(responseBody.version),
    };
  } catch (error) {
    if (error instanceof FactoryDefinitionAPIError) {
      throw new CurrentEditableFactoryDefinitionError(
        `The current factory editing API returned a factory definition the dashboard cannot edit. ${error.message}`,
        {
          cause: error,
          code: "INVALID_FACTORY_DEFINITION",
          responseBody,
          ...responseDetails,
        },
      );
    }

    throw error;
  }
}

function normalizeEditableFactoryDefinitionVersion(
  value: EditableFactoryDefinition["version"],
): EditableFactoryDefinitionVersion {
  if (
    !value ||
    typeof value.logical !== "number" ||
    !Number.isFinite(value.logical) ||
    typeof value.physical !== "string"
  ) {
    throw new CurrentEditableFactoryDefinitionError(
      "The current factory editing API returned an invalid response.",
      {
        code: "INTERNAL_ERROR",
        responseBody: value,
      },
    );
  }

  return {
    logical: value.logical,
    physical: value.physical,
  };
}

async function readResponseBody(response: Response): Promise<unknown> {
  const rawBody = await response.text();
  if (rawBody.length === 0) {
    return null;
  }

  try {
    return JSON.parse(rawBody) as unknown;
  } catch {
    return rawBody;
  }
}

function asAPIErrorResponse(value: unknown): APIErrorResponse | null {
  if (!isRecord(value)) {
    return null;
  }

  return {
    code: typeof value.code === "string" ? value.code : undefined,
    message: typeof value.message === "string" ? value.message : undefined,
  };
}

function normalizeCurrentEditableFactoryDefinitionErrorCode(
  code: string | undefined,
): CurrentEditableFactoryDefinitionErrorCode {
  switch (code) {
    case "NOT_FOUND":
      return code;
    default:
      return "INTERNAL_ERROR";
  }
}

function isEditableFactoryDefinitionValue(
  value: unknown,
): value is EditableFactoryDefinition {
  return (
    isRecord(value) &&
    value.factoryDefinition !== undefined &&
    isRecord(value.version)
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
