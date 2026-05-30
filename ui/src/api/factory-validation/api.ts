import type { components } from "../generated/openapi";
import { factoryAPIURL } from "../baseUrl";
import type { CanonicalFactoryDefinition } from "../factory-definition";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import { factoryValidationAPIErrorMessages } from "./messages";

type Factory = components["schemas"]["Factory"];

export type FactoryValidationTarget =
  components["schemas"]["FactoryValidationTarget"];
export type FactoryValidationResult =
  components["schemas"]["FactoryValidationResult"];

export type FactoryValidationAPIErrorCode =
  | "BAD_REQUEST"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export interface FactoryValidationAPIErrorDetails {
  code: FactoryValidationAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export interface ValidateFactoryDefinitionOptions {
  fetch?: typeof globalThis.fetch;
  signal?: AbortSignal;
}

export class FactoryValidationAPIError extends Error {
  public readonly code: FactoryValidationAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: FactoryValidationAPIErrorDetails,
  ) {
    super(message);
    this.name = "FactoryValidationAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

const VALIDATE_FACTORY_ENDPOINT = "/factory-validations";

export async function validateFactoryDefinition(
  factoryDefinition: CanonicalFactoryDefinition,
  options: ValidateFactoryDefinitionOptions = {},
): Promise<FactoryValidationResult> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new FactoryValidationAPIError(
      factoryValidationAPIErrorMessages.emptyEnvironment,
      { code: "NETWORK_ERROR" },
    );
  }

  const requestBody: Factory = {
    ...factoryDefinition,
  };

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(VALIDATE_FACTORY_ENDPOINT),
      {
        body: JSON.stringify(requestBody),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
        signal: options.signal,
      },
    );
  } catch (error) {
    throw new FactoryValidationAPIError(
      factoryValidationAPIErrorMessages.network,
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    throw new FactoryValidationAPIError(
      resolveFactoryValidationErrorMessage(responseBody),
      {
        code: resolveFactoryValidationErrorCode(response.status),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isFactoryValidationResult(responseBody)) {
    throw new FactoryValidationAPIError(
      factoryValidationAPIErrorMessages.invalidResponse,
      {
        // hardcoded-ui-copy-exception: non-product-diagnostic
        code: "INTERNAL_ERROR",
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  return responseBody;
}

function resolveFactoryValidationErrorCode(
  status: number,
): FactoryValidationAPIErrorCode {
  if (status === 400) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return "BAD_REQUEST";
  }

  // hardcoded-ui-copy-exception: non-product-diagnostic
  return "INTERNAL_ERROR";
}

function resolveFactoryValidationErrorMessage(responseBody: unknown): string {
  const payload = extractAPIErrorPayload(responseBody);
  if (
    payload &&
    typeof payload.message === "string" &&
    payload.message.trim().length > 0
  ) {
    return payload.message;
  }

  return factoryValidationAPIErrorMessages.rejectedRequest;
}

function isFactoryValidationResult(
  value: unknown,
): value is FactoryValidationResult {
  if (!isAPIRecord(value) || !Array.isArray(value.targets)) {
    return false;
  }

  return value.targets.every(isFactoryValidationTarget);
}

function isFactoryValidationTarget(value: unknown): value is FactoryValidationTarget {
  if (!isAPIRecord(value)) {
    return false;
  }

  return (
    typeof value.code === "string" &&
    typeof value.message === "string" &&
    typeof value.severity === "string" &&
    isAPIRecord(value.subject) &&
    typeof value.subject.id === "string" &&
    typeof value.subject.location === "string" &&
    typeof value.subject.type === "string"
  );
}
