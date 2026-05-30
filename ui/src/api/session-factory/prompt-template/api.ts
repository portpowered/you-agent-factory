import { factoryAPIURL } from "../../baseUrl";
import type { components } from "../../generated/openapi";
import { currentFactoryWorkstationPath } from "../../session-routing";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../../transport";
import { promptTemplateAPIErrorMessages } from "./messages";

export type PromptTemplateContract =
  components["schemas"]["PromptTemplateContract"];
export type PromptTemplateValidationResult =
  components["schemas"]["PromptTemplateValidationResult"];

export type CurrentFactoryPromptTemplateAPIErrorCode =
  | "BAD_REQUEST"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR"
  | "NOT_FOUND";

export interface CurrentFactoryPromptTemplateAPIErrorDetails {
  code: CurrentFactoryPromptTemplateAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export interface CurrentFactoryPromptTemplateOptions {
  fetch?: typeof globalThis.fetch;
  sessionID?: string | null;
}

export class CurrentFactoryPromptTemplateAPIError extends Error {
  public readonly code: CurrentFactoryPromptTemplateAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: CurrentFactoryPromptTemplateAPIErrorDetails,
  ) {
    super(message);
    this.name = "CurrentFactoryPromptTemplateAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

export async function getCurrentFactoryWorkstationPromptTemplateContract(
  workstationName: string,
  options: CurrentFactoryPromptTemplateOptions = {},
): Promise<PromptTemplateContract> {
  return fetchPromptTemplateJSON(
    currentFactoryWorkstationPath(
      workstationName,
      options.sessionID,
      "prompt-template-contract",
    ),
    {
      emptyEnvironmentMessage:
        promptTemplateAPIErrorMessages.contractEmptyEnvironment,
      invalidResponseMessage:
        promptTemplateAPIErrorMessages.contractInvalidResponse,
      networkMessage: promptTemplateAPIErrorMessages.contractNetwork,
      options,
    },
  );
}

export async function validateCurrentFactoryWorkstationPromptTemplate(
  workstationName: string,
  prompt: string,
  options: CurrentFactoryPromptTemplateOptions = {},
): Promise<PromptTemplateValidationResult> {
  return fetchPromptTemplateJSON(
    currentFactoryWorkstationPath(
      workstationName,
      options.sessionID,
      "prompt-template-validation",
    ),
    {
      body: JSON.stringify({ prompt }),
      emptyEnvironmentMessage:
        promptTemplateAPIErrorMessages.validationEmptyEnvironment,
      invalidResponseMessage:
        promptTemplateAPIErrorMessages.validationInvalidResponse,
      method: "POST",
      networkMessage: promptTemplateAPIErrorMessages.validationNetwork,
      options,
    },
  );
}

async function fetchPromptTemplateJSON<T>(
  endpoint: string,
  config: {
    body?: string;
    emptyEnvironmentMessage: string;
    invalidResponseMessage: string;
    method?: "GET" | "POST";
    networkMessage: string;
    options: CurrentFactoryPromptTemplateOptions;
  },
): Promise<T> {
  const fetchImplementation = config.options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new CurrentFactoryPromptTemplateAPIError(
      config.emptyEnvironmentMessage,
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(factoryAPIURL(endpoint), {
      body: config.body,
      headers: config.body
        ? {
            "Content-Type": "application/json",
          }
        : undefined,
      method: config.method ?? "GET",
    });
  } catch (error) {
    throw new CurrentFactoryPromptTemplateAPIError(config.networkMessage, {
      code: "NETWORK_ERROR",
      responseBody: error,
    });
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorBody = extractAPIErrorPayload(responseBody);
    throw new CurrentFactoryPromptTemplateAPIError(
      errorBody?.message ??
        promptTemplateAPIErrorMessages.genericRejectedRequest,
      {
        code: normalizePromptTemplateAPIErrorCode(errorBody?.code),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isAPIRecord(responseBody)) {
    throw new CurrentFactoryPromptTemplateAPIError(
      config.invalidResponseMessage,
      {
        code: "INTERNAL_ERROR",
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  return responseBody as T;
}

function normalizePromptTemplateAPIErrorCode(
  code: string | undefined,
): CurrentFactoryPromptTemplateAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "NOT_FOUND":
      return code;
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}
