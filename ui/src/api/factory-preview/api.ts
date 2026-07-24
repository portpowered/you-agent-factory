import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import { factoryPreviewAPIErrorMessages } from "./messages";

export type FactoryPreviewRequest =
  components["schemas"]["FactoryPreviewRequest"];
export type FactoryPreviewResult =
  components["schemas"]["FactoryPreviewResult"];
export type FactoryPreviewDiagnostic =
  components["schemas"]["WorkflowDiagnostic"];

export type FactoryPreviewAPIErrorCode =
  | "BAD_REQUEST"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export interface FactoryPreviewAPIErrorDetails {
  code: FactoryPreviewAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export interface FactoryPreviewOptions {
  fetch?: typeof globalThis.fetch;
  signal?: AbortSignal;
}

export class FactoryPreviewAPIError extends Error {
  public readonly code: FactoryPreviewAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(message: string, details: FactoryPreviewAPIErrorDetails) {
    super(message);
    this.name = "FactoryPreviewAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

const FACTORY_PREVIEW_ENDPOINT = "/factories/preview";

export async function previewFactory(
  request: FactoryPreviewRequest,
  options: FactoryPreviewOptions = {},
): Promise<FactoryPreviewResult> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new FactoryPreviewAPIError(
      factoryPreviewAPIErrorMessages.emptyEnvironment,
      { code: "NETWORK_ERROR" },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(FACTORY_PREVIEW_ENDPOINT),
      {
        body: JSON.stringify(request),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
        signal: options.signal,
      },
    );
  } catch (error) {
    throw new FactoryPreviewAPIError(factoryPreviewAPIErrorMessages.network, {
      code: "NETWORK_ERROR",
      responseBody: error,
    });
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    throw new FactoryPreviewAPIError(
      resolveFactoryPreviewErrorMessage(responseBody),
      {
        code: resolveFactoryPreviewErrorCode(response.status),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isFactoryPreviewResult(responseBody)) {
    throw new FactoryPreviewAPIError(
      factoryPreviewAPIErrorMessages.invalidResponse,
      {
        code: "INTERNAL_ERROR",
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  return responseBody;
}

function resolveFactoryPreviewErrorCode(
  status: number,
): FactoryPreviewAPIErrorCode {
  if (status === 400) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return "BAD_REQUEST";
  }

  // hardcoded-ui-copy-exception: non-product-diagnostic
  return "INTERNAL_ERROR";
}

function resolveFactoryPreviewErrorMessage(responseBody: unknown): string {
  const payload = extractAPIErrorPayload(responseBody);
  if (
    payload &&
    typeof payload.message === "string" &&
    payload.message.trim().length > 0
  ) {
    return payload.message;
  }

  return factoryPreviewAPIErrorMessages.rejectedRequest;
}

function isFactoryPreviewResult(value: unknown): value is FactoryPreviewResult {
  if (!isAPIRecord(value)) {
    return false;
  }

  return (
    typeof value.valid === "boolean" &&
    isAPIRecord(value.sourceResolution) &&
    Array.isArray(value.sourceValidationIssues) &&
    isAPIRecord(value.policyPreview) &&
    isAPIRecord(value.resultConstraints)
  );
}
