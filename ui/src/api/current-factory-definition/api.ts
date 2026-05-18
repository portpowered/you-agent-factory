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
type ErrorTarget = components["schemas"]["ErrorTarget"];
type SaveEditableFactoryDefinitionRequest =
  components["schemas"]["SaveEditableFactoryDefinitionRequest"];

export interface EditableFactoryDefinitionDocument {
  factoryDefinition: CanonicalFactoryDefinition;
  version: EditableFactoryDefinitionVersion;
}

export type CurrentEditableFactoryDefinitionErrorCode =
  | "BAD_REQUEST"
  | "FACTORY_NOT_IDLE"
  | "INTERNAL_ERROR"
  | "INVALID_FACTORY_DEFINITION"
  | "NETWORK_ERROR"
  | "NOT_FOUND"
  | "STALE_FACTORY_VERSION";

export interface CurrentEditableFactoryDefinitionErrorDetails {
  cause?: unknown;
  code: CurrentEditableFactoryDefinitionErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
  targets?: ErrorTarget[];
}

export interface GetCurrentEditableFactoryDefinitionOptions {
  fetch?: typeof globalThis.fetch;
}

interface APIErrorResponse {
  code?: string;
  message?: string;
  targets?: ErrorTarget[];
}

export interface SaveCurrentEditableFactoryDefinitionOptions {
  fetch?: typeof globalThis.fetch;
}

export interface SaveCurrentEditableFactoryDefinitionInput {
  baseVersion?: EditableFactoryDefinitionVersion;
  factoryDefinition: CanonicalFactoryDefinition;
}

const GET_CURRENT_EDITABLE_FACTORY_DEFINITION_ENDPOINT =
  "/factory/~current/editable-definition";

export class CurrentEditableFactoryDefinitionError extends Error {
  public readonly cause?: unknown;
  public readonly code: CurrentEditableFactoryDefinitionErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;
  public readonly targets?: ErrorTarget[];

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
    this.targets = details.targets;
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
        targets: errorBody?.targets,
      },
    );
  }

  return normalizeEditableFactoryDefinitionDocument(responseBody, {
    status: response.status,
    statusText: response.statusText,
  });
}

export async function saveCurrentEditableFactoryDefinitionDocument(
  input: SaveCurrentEditableFactoryDefinitionInput,
  options: SaveCurrentEditableFactoryDefinitionOptions = {},
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

  const requestBody: SaveEditableFactoryDefinitionRequest = {
    baseVersion: input.baseVersion,
    factoryDefinition: input.factoryDefinition,
  };

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(GET_CURRENT_EDITABLE_FACTORY_DEFINITION_ENDPOINT),
      {
        body: JSON.stringify(requestBody),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
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
        "The current factory editing API rejected the save request.",
      {
        code: normalizeCurrentEditableFactoryDefinitionErrorCode(
          errorBody?.code,
        ),
        responseBody,
        status: response.status,
        statusText: response.statusText,
        targets: errorBody?.targets,
      },
    );
  }

  return normalizeEditableFactoryDefinitionDocument(responseBody, {
    status: response.status,
    statusText: response.statusText,
  });
}

export async function saveCurrentEditableFactoryDefinition(
  input: SaveCurrentEditableFactoryDefinitionInput,
  options: SaveCurrentEditableFactoryDefinitionOptions = {},
): Promise<CanonicalFactoryDefinition> {
  const document = await saveCurrentEditableFactoryDefinitionDocument(
    input,
    options,
  );
  return document.factoryDefinition;
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
    targets: Array.isArray(value.targets)
      ? value.targets.filter(isErrorTarget)
      : undefined,
  };
}

function normalizeCurrentEditableFactoryDefinitionErrorCode(
  code: string | undefined,
): CurrentEditableFactoryDefinitionErrorCode {
  switch (code) {
    case "BAD_REQUEST":
      return code;
    case "NOT_FOUND":
      return code;
    case "FACTORY_NOT_IDLE":
      return code;
    case "STALE_FACTORY_VERSION":
      return code;
    case "INVALID_FACTORY":
      return "INVALID_FACTORY_DEFINITION";
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

function isErrorTarget(value: unknown): value is ErrorTarget {
  return isRecord(value) && typeof value.kind === "string";
}
