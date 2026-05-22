import { factoryAPIURL } from "../baseUrl";
import type { components, operations } from "../generated/openapi";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
export type ProviderSessionDetailResponse =
  components["schemas"]["ProviderSessionDetailResponse"];
export type ProviderSessionDetailRef =
  operations["getProviderSessionDetails"]["parameters"]["query"];

export type ProviderSessionDetailsAPIErrorCode =
  | "BAD_REQUEST"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR"
  | "NOT_FOUND";

export interface GetProviderSessionDetailsOptions {
  fetch?: typeof globalThis.fetch;
}

const GET_PROVIDER_SESSION_DETAIL_ENDPOINT = "/provider-sessions/detail";
const LOADABLE_PROVIDER_SESSION_ID_PATTERN = /^[A-Za-z0-9_-]+$/;
const LOADABLE_PROVIDER_SESSION_PROVIDER: ProviderSessionDetailRef["provider"] =
  "codex";
const LOADABLE_PROVIDER_SESSION_KIND: ProviderSessionDetailRef["kind"] =
  "session_id";

export class ProviderSessionDetailsAPIError extends Error {
  public readonly code: ProviderSessionDetailsAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: {
      code: ProviderSessionDetailsAPIErrorCode;
      responseBody?: unknown;
      status?: number;
      statusText?: string;
    },
  ) {
    super(message);
    this.name = "ProviderSessionDetailsAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

export async function getProviderSessionDetails(
  session: ProviderSessionDetailRef,
  options: GetProviderSessionDetailsOptions = {},
): Promise<ProviderSessionDetailResponse> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new ProviderSessionDetailsAPIError(
      "Provider-session details are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(
        `${GET_PROVIDER_SESSION_DETAIL_ENDPOINT}?${new URLSearchParams({
          id: session.id,
          kind: session.kind,
          provider: session.provider,
        }).toString()}`,
      ),
      {
        method: "GET",
      },
    );
  } catch (error) {
    throw new ProviderSessionDetailsAPIError(
      "The dashboard could not reach the provider-session detail API.",
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorBody = extractAPIErrorPayload(responseBody);
    throw new ProviderSessionDetailsAPIError(
      errorBody?.message ??
        "The provider-session detail API rejected the request.",
      {
        code: normalizeProviderSessionDetailsAPIErrorCode(errorBody?.code),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isProviderSessionDetailResponse(responseBody)) {
    throw new ProviderSessionDetailsAPIError(
      "The provider-session detail API returned an invalid response.",
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

export function toProviderSessionDetailRef(session: {
  id?: string | null;
  kind?: string | null;
  provider?: string | null;
}): ProviderSessionDetailRef | null {
  const provider = normalizeProviderSessionPart(session.provider);
  const kind = normalizeProviderSessionPart(session.kind);
  const id = normalizeProviderSessionID(session.id);

  if (
    provider !== LOADABLE_PROVIDER_SESSION_PROVIDER ||
    kind !== LOADABLE_PROVIDER_SESSION_KIND ||
    id === null
  ) {
    return null;
  }

  return {
    id,
    kind,
    provider,
  };
}

function normalizeProviderSessionDetailsAPIErrorCode(
  code: string | undefined,
): ProviderSessionDetailsAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "NOT_FOUND":
      return code;
    default:
      return "INTERNAL_ERROR";
  }
}

function isProviderSessionDetailResponse(
  value: unknown,
): value is ProviderSessionDetailResponse {
  return (
    isAPIRecord(value) &&
    isAPIRecord(value.providerSession) &&
    isAPIRecord(value.source) &&
    isAPIRecord(value.parse) &&
    Array.isArray(value.transcript) &&
    Array.isArray(value.parse.turns) &&
    Array.isArray(value.parse.functionCalls) &&
    Array.isArray(value.parse.reasoning) &&
    Array.isArray(value.parse.parseErrors) &&
    Array.isArray(value.parse.unknownEvents)
  );
}

function normalizeProviderSessionID(value: string | null | undefined): string | null {
  const trimmed = value?.trim();
  return trimmed && LOADABLE_PROVIDER_SESSION_ID_PATTERN.test(trimmed)
    ? trimmed
    : null;
}

function normalizeProviderSessionPart(value: string | null | undefined): string | null {
  const trimmed = value?.trim().toLowerCase();
  return trimmed && trimmed.length > 0 ? trimmed : null;
}
