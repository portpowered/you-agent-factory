import { factoryAPIURL } from "../baseUrl";
import type { components, operations } from "../generated/openapi";
import { isAPIRecord, readAPIResponseBody } from "../transport";
import { buildFactorySessionsAPIError, FactorySessionsAPIError } from "./api";

export type FactoryDispatch = components["schemas"]["FactoryDispatch"];
export type FactorySessionDispatchDetailRef =
  operations["getFactorySessionDispatch"]["parameters"]["path"];

export interface GetFactorySessionDispatchDetailOptions {
  fetch?: typeof globalThis.fetch;
}

const FACTORY_SESSIONS_ENDPOINT = "/factory-sessions";

export async function getFactorySessionDispatchDetail(
  detailRef: FactorySessionDispatchDetailRef,
  options: GetFactorySessionDispatchDetailOptions = {},
): Promise<FactoryDispatch> {
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
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(detailRef.session_id)}/dispatches/${encodeURIComponent(detailRef.dispatch_id)}`,
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

  if (!isFactoryDispatch(responseBody)) {
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

  return responseBody;
}

function isFactoryDispatch(value: unknown): value is FactoryDispatch {
  return (
    isAPIRecord(value) &&
    typeof value.id === "string" &&
    typeof value.sessionId === "string" &&
    typeof value.dispatchKind === "string" &&
    typeof value.status === "string" &&
    typeof value.orchestratorKind === "string"
  );
}
