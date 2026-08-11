import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import { readAPIResponseBody } from "../transport";

export type PackagedFactoryCatalogResponse =
  components["schemas"]["PackagedFactoryCatalogResponse"];
export type PackagedFactoryCatalogEntry =
  components["schemas"]["PackagedFactoryCatalogEntry"];

export type PackagedFactoryCatalogAPIErrorCode =
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export interface PackagedFactoryCatalogAPIErrorDetails {
  readonly code: PackagedFactoryCatalogAPIErrorCode;
  readonly responseBody?: unknown;
  readonly status?: number;
  readonly statusText?: string;
}

export interface PackagedFactoryCatalogOptions {
  readonly fetch?: typeof globalThis.fetch;
  readonly signal?: AbortSignal;
}

export class PackagedFactoryCatalogAPIError extends Error {
  public readonly code: PackagedFactoryCatalogAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: PackagedFactoryCatalogAPIErrorDetails,
  ) {
    super(message);
    this.name = "PackagedFactoryCatalogAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

const PACKAGED_FACTORY_CATALOG_ENDPOINT = "/packaged-factories";

export async function getPackagedFactoryCatalog(
  options: PackagedFactoryCatalogOptions = {},
): Promise<PackagedFactoryCatalogResponse> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new PackagedFactoryCatalogAPIError("NETWORK_ERROR", {
      code: "NETWORK_ERROR",
    });
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(PACKAGED_FACTORY_CATALOG_ENDPOINT),
      {
        headers: { Accept: "application/json" },
        method: "GET",
        signal: options.signal,
      },
    );
  } catch (error) {
    throw new PackagedFactoryCatalogAPIError("NETWORK_ERROR", {
      code: "NETWORK_ERROR",
      responseBody: error,
    });
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    throw new PackagedFactoryCatalogAPIError("INTERNAL_ERROR", {
      code: "INTERNAL_ERROR",
      responseBody,
      status: response.status,
      statusText: response.statusText,
    });
  }

  if (!isPackagedFactoryCatalogResponse(responseBody)) {
    throw new PackagedFactoryCatalogAPIError("INTERNAL_ERROR", {
      code: "INTERNAL_ERROR",
      responseBody,
      status: response.status,
      statusText: response.statusText,
    });
  }

  return responseBody;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNameValue(value: unknown): boolean {
  if (!isRecord(value)) {
    return false;
  }

  return (
    value.type === "LOCALIZABLE_ASSET" &&
    typeof value.value === "string" &&
    value.value.trim().length > 0 &&
    (value.id === undefined || typeof value.id === "string") &&
    (value.locales === undefined ||
      (Array.isArray(value.locales) &&
        value.locales.every((locale) => typeof locale === "string"))) &&
    (value.values === undefined ||
      (isRecord(value.values) &&
        Object.values(value.values).every(
          (localizedValue) => typeof localizedValue === "string",
        )))
  );
}

function isInvocationArguments(value: unknown): boolean {
  return (
    isRecord(value) &&
    Object.values(value).every(
      (argument) =>
        typeof argument === "string" ||
        (Array.isArray(argument) &&
          argument.every((item) => typeof item === "string")),
    )
  );
}

function isInvocationExample(value: unknown): boolean {
  return (
    isRecord(value) &&
    typeof value.name === "string" &&
    value.name.trim().length > 0 &&
    isNameValue(value.description) &&
    isInvocationArguments(value.args)
  );
}

function isCatalogEntry(value: unknown): value is PackagedFactoryCatalogEntry {
  return (
    isRecord(value) &&
    typeof value.name === "string" &&
    typeof value.project === "string" &&
    typeof value.slug === "string" &&
    isNameValue(value.description) &&
    Array.isArray(value.examples) &&
    value.examples.every(isInvocationExample) &&
    isRecord(value.json) &&
    typeof value.yaml === "string"
  );
}

function isPackagedFactoryCatalogResponse(
  value: unknown,
): value is PackagedFactoryCatalogResponse {
  return (
    isRecord(value) &&
    Array.isArray(value.factories) &&
    value.factories.every(isCatalogEntry)
  );
}
