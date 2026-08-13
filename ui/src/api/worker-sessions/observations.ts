import { factoryAPIURL } from "../baseUrl";
import { factorySessionScopedPath } from "../session-routing";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import type { WorkerSessionObservation } from "./types";

export interface ListWorkerSessionsForWorkOptions {
  factorySessionID: string;
  workID: string;
  fetch?: typeof globalThis.fetch;
  signal?: AbortSignal;
}

export type WorkerSessionObservationAPIErrorCode =
  | "INVALID_RESPONSE"
  | "NETWORK_ERROR"
  | "REQUEST_FAILED";

export class WorkerSessionObservationAPIError extends Error {
  public readonly code: WorkerSessionObservationAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;

  public constructor(
    message: string,
    details: {
      code: WorkerSessionObservationAPIErrorCode;
      responseBody?: unknown;
      status?: number;
    },
  ) {
    super(message);
    this.name = "WorkerSessionObservationAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
  }
}

export async function listWorkerSessionsForWork({
  factorySessionID,
  workID,
  fetch: fetchImplementation = globalThis.fetch,
  signal,
}: ListWorkerSessionsForWorkOptions): Promise<WorkerSessionObservation[]> {
  if (typeof fetchImplementation !== "function") {
    throw new WorkerSessionObservationAPIError(
      "Worker Session observations are unavailable in this environment.",
      { code: "NETWORK_ERROR" },
    );
  }

  const path = factorySessionScopedPath("/worker-sessions", factorySessionID);
  const url = `${factoryAPIURL(path)}?workId=${encodeURIComponent(workID)}`;
  let response: Response;
  try {
    response = await fetchImplementation(url, {
      method: "GET",
      ...(signal ? { signal } : {}),
    });
  } catch (error) {
    throw new WorkerSessionObservationAPIError(
      "The dashboard could not reach the Worker Sessions API.",
      { code: "NETWORK_ERROR", responseBody: error },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    const errorPayload = extractAPIErrorPayload(responseBody);
    throw new WorkerSessionObservationAPIError(
      errorPayload?.message ??
        "The Worker Sessions API rejected the observation request.",
      {
        code: "REQUEST_FAILED",
        responseBody,
        status: response.status,
      },
    );
  }

  if (
    !isAPIRecord(responseBody) ||
    !Array.isArray(responseBody.sessions) ||
    !responseBody.sessions.every(isWorkerSessionObservation)
  ) {
    throw new WorkerSessionObservationAPIError(
      "The Worker Sessions API returned an invalid observation response.",
      {
        code: "INVALID_RESPONSE",
        responseBody,
        status: response.status,
      },
    );
  }

  return responseBody.sessions;
}

function isWorkerSessionObservation(
  value: unknown,
): value is WorkerSessionObservation {
  return (
    isAPIRecord(value) &&
    typeof value.workerSessionId === "string" &&
    value.workerSessionId.length > 0 &&
    typeof value.direct === "boolean" &&
    Array.isArray(value.workIds) &&
    value.workIds.every(isString) &&
    typeof value.attemptId === "string" &&
    value.attemptId.length > 0 &&
    typeof value.state === "string" &&
    (value.startedAt === null || typeof value.startedAt === "string") &&
    (value.endedAt === null || typeof value.endedAt === "string") &&
    (value.durationMillis === null ||
      typeof value.durationMillis === "number") &&
    typeof value.durationBasis === "string" &&
    typeof value.transcript === "string" &&
    isAPIRecord(value.parse)
  );
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}
