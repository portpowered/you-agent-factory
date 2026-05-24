import type { components } from "../generated/openapi";
import { factoryAPIURL } from "../baseUrl";
import { currentFactorySessionPath } from "../session-routing";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";

export type FactoryValue = components["schemas"]["Factory"];

export type NamedFactoryAPIErrorCode =
  | "BAD_REQUEST"
  | "FACTORY_ALREADY_EXISTS"
  | "FACTORY_NOT_IDLE"
  | "INTERNAL_ERROR"
  | "INVALID_FACTORY"
  | "INVALID_FACTORY_NAME"
  | "NETWORK_ERROR"
  | "NOT_FOUND";

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

function normalizeNamedFactoryAPIErrorCode(code: string | undefined): NamedFactoryAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "FACTORY_ALREADY_EXISTS":
    case "FACTORY_NOT_IDLE":
    case "INTERNAL_ERROR":
    case "INVALID_FACTORY":
    case "INVALID_FACTORY_NAME":
    case "NOT_FOUND":
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
