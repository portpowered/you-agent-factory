import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import { isAPIRecord, readAPIResponseBody } from "../transport";
import {
  buildFactorySessionsAPIError,
  FACTORY_SESSIONS_ENDPOINT,
  FactorySessionsAPIError,
} from "./api";

export type FactorySessionLifecycleControlRequest =
  components["schemas"]["FactorySessionLifecycleControlRequest"];
export type FactorySessionApproveRequest =
  components["schemas"]["FactorySessionApproveRequest"];
export type FactorySessionRetryDispatchRequest =
  components["schemas"]["FactorySessionRetryDispatchRequest"];
export type FactorySessionInterruptDispatchRequest =
  components["schemas"]["FactorySessionInterruptDispatchRequest"];
export type FactorySessionLifecycleControlResponse =
  components["schemas"]["FactorySessionLifecycleControlResponse"];

interface FactorySessionLifecycleControlOptions {
  fetch?: typeof globalThis.fetch;
}

export async function approveFactorySession(
  sessionID: string,
  request: FactorySessionApproveRequest = {},
  options: FactorySessionLifecycleControlOptions = {},
): Promise<FactorySessionLifecycleControlResponse> {
  return postFactorySessionLifecycleControl(
    sessionID,
    "/approve",
    request,
    options,
  );
}

export async function pauseFactorySession(
  sessionID: string,
  request: FactorySessionLifecycleControlRequest = {},
  options: FactorySessionLifecycleControlOptions = {},
): Promise<FactorySessionLifecycleControlResponse> {
  return postFactorySessionLifecycleControl(
    sessionID,
    "/pause",
    request,
    options,
  );
}

export async function resumeFactorySession(
  sessionID: string,
  request: FactorySessionLifecycleControlRequest = {},
  options: FactorySessionLifecycleControlOptions = {},
): Promise<FactorySessionLifecycleControlResponse> {
  return postFactorySessionLifecycleControl(
    sessionID,
    "/resume",
    request,
    options,
  );
}

export async function cancelFactorySession(
  sessionID: string,
  request: FactorySessionLifecycleControlRequest = {},
  options: FactorySessionLifecycleControlOptions = {},
): Promise<FactorySessionLifecycleControlResponse> {
  return postFactorySessionLifecycleControl(
    sessionID,
    "/cancel",
    request,
    options,
  );
}

export async function terminateFactorySession(
  sessionID: string,
  request: FactorySessionLifecycleControlRequest = {},
  options: FactorySessionLifecycleControlOptions = {},
): Promise<FactorySessionLifecycleControlResponse> {
  return postFactorySessionLifecycleControl(
    sessionID,
    "/terminate",
    request,
    options,
  );
}

export async function retryFactorySessionDispatch(
  sessionID: string,
  request: FactorySessionRetryDispatchRequest,
  options: FactorySessionLifecycleControlOptions = {},
): Promise<FactorySessionLifecycleControlResponse> {
  return postFactorySessionLifecycleControl(
    sessionID,
    "/retry-dispatch",
    request,
    options,
  );
}

export async function interruptFactorySessionDispatch(
  sessionID: string,
  request: FactorySessionInterruptDispatchRequest,
  options: FactorySessionLifecycleControlOptions = {},
): Promise<FactorySessionLifecycleControlResponse> {
  return postFactorySessionLifecycleControl(
    sessionID,
    "/interrupt-dispatch",
    request,
    options,
  );
}

async function postFactorySessionLifecycleControl(
  sessionID: string,
  actionPath: string,
  request:
    | FactorySessionApproveRequest
    | FactorySessionInterruptDispatchRequest
    | FactorySessionLifecycleControlRequest
    | FactorySessionRetryDispatchRequest,
  options: FactorySessionLifecycleControlOptions,
): Promise<FactorySessionLifecycleControlResponse> {
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
        `${FACTORY_SESSIONS_ENDPOINT}/${encodeURIComponent(sessionID)}${actionPath}`,
      ),
      {
        body: JSON.stringify(request),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
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
  if (!response.ok && response.status !== 409) {
    throw buildFactorySessionsAPIError(
      response,
      responseBody,
      "The factory sessions API rejected the lifecycle control request.",
    );
  }

  if (!isFactorySessionLifecycleControlResponse(responseBody)) {
    throw new FactorySessionsAPIError(
      "The factory sessions API returned an invalid lifecycle control response.",
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

function isFactorySessionLifecycleControlResponse(
  value: unknown,
): value is FactorySessionLifecycleControlResponse {
  if (!isAPIRecord(value)) {
    return false;
  }

  return (
    typeof value.sessionId === "string" &&
    typeof value.operation === "string" &&
    typeof value.outcome === "string" &&
    typeof value.status === "string"
  );
}
