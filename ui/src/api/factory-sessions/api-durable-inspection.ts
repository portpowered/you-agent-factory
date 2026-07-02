import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import { isAPIRecord, readAPIResponseBody } from "../transport";
import {
  buildFactorySessionsAPIError,
  FACTORY_SESSIONS_ENDPOINT,
  FactorySessionsAPIError,
} from "./api";
import {
  type DurableResultSurfaces,
  resultSurfacesFromDurableResult,
} from "./normalize-durable-inspection";

export type FactorySessionResult =
  components["schemas"]["FactorySessionResult"];
export type FactorySessionResultMode =
  components["schemas"]["FactorySessionResultMode"];
export type ListFactorySessionDispatchesResponse =
  components["schemas"]["ListFactorySessionDispatchesResponse"];

export interface GetFactorySessionDurableResultsOptions {
  fetch?: typeof globalThis.fetch;
}

export interface ListFactorySessionDispatchesOptions {
  fetch?: typeof globalThis.fetch;
}

export async function listFactorySessionDispatches(
  sessionID: string,
  options: ListFactorySessionDispatchesOptions = {},
): Promise<ListFactorySessionDispatchesResponse> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new FactorySessionsAPIError(
      "Factory sessions are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}/dispatches`,
      ),
      {
        method: "GET",
      },
    );
  } catch (error) {
    throw new FactorySessionsAPIError(
      "The dashboard could not reach the factory sessions API.",
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    throw buildFactorySessionsAPIError(
      response,
      responseBody,
      "The factory sessions API rejected the request.",
    );
  }

  if (
    !isAPIRecord(responseBody) ||
    typeof responseBody.sessionId !== "string" ||
    !Array.isArray(responseBody.dispatches)
  ) {
    throw new FactorySessionsAPIError(
      "The factory sessions API returned an invalid response.",
      {
        code: "INTERNAL_ERROR",
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  return responseBody as ListFactorySessionDispatchesResponse;
}

export async function getFactorySessionDurableResults(
  sessionID: string,
  mode: FactorySessionResultMode,
  options: GetFactorySessionDurableResultsOptions = {},
): Promise<FactorySessionResult> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new FactorySessionsAPIError(
      "Factory sessions are unavailable in this environment.",
      {
        code: "NETWORK_ERROR",
      },
    );
  }

  const query = new URLSearchParams({ mode });
  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}/results?${query.toString()}`,
      ),
      {
        method: "GET",
      },
    );
  } catch (error) {
    throw new FactorySessionsAPIError(
      "The dashboard could not reach the factory sessions API.",
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    throw buildFactorySessionsAPIError(
      response,
      responseBody,
      "The factory sessions API rejected the request.",
    );
  }

  if (
    !isAPIRecord(responseBody) ||
    typeof responseBody.sessionId !== "string"
  ) {
    throw new FactorySessionsAPIError(
      "The factory sessions API returned an invalid response.",
      {
        code: "INTERNAL_ERROR",
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  return responseBody as FactorySessionResult;
}

export function durableResultSurfacesFromResultsResponse(
  durableResult: FactorySessionResult,
  fallbackPhase?: string,
): DurableResultSurfaces {
  return resultSurfacesFromDurableResult(durableResult, fallbackPhase);
}
