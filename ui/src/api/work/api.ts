import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import { factorySessionScopedPath } from "../session-routing";
import { extractAPIErrorPayload, readAPIResponseBody } from "../transport";

type SubmitWorkRequest = components["schemas"]["SubmitWorkRequest"];
type SubmitWorkResponse = components["schemas"]["SubmitWorkResponse"];
type StageSubmitWorkFileRequest =
  components["schemas"]["StageSubmitWorkFileRequest"];
type StageSubmitWorkFileResponse =
  components["schemas"]["StageSubmitWorkFileResponse"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

const SUBMIT_WORK_ENDPOINT = "/work";
const STAGE_SUBMIT_WORK_FILE_ENDPOINT = "/work/staged-files";
const GENERIC_SUBMIT_WORK_ERROR_MESSAGE =
  "Dashboard submission failed. Try again in a moment.";
const GENERIC_STAGE_SUBMIT_WORK_FILE_ERROR_MESSAGE =
  "We couldn't stage that file. Try again in a moment.";

export class SubmitWorkAPIError extends Error {
  public readonly code?: ErrorResponse["code"];
  public readonly status: number;
  public readonly statusText: string;

  constructor({
    code,
    message,
    status,
    statusText,
  }: {
    code?: ErrorResponse["code"];
    message: string;
    status: number;
    statusText: string;
  }) {
    super(message);
    this.name = "SubmitWorkAPIError";
    this.code = code;
    this.status = status;
    this.statusText = statusText;
  }
}

export class StageSubmitWorkFileAPIError extends Error {
  public readonly code?: ErrorResponse["code"];
  public readonly status: number;
  public readonly statusText: string;

  constructor({
    code,
    message,
    status,
    statusText,
  }: {
    code?: ErrorResponse["code"];
    message: string;
    status: number;
    statusText: string;
  }) {
    super(message);
    this.name = "StageSubmitWorkFileAPIError";
    this.code = code;
    this.status = status;
    this.statusText = statusText;
  }
}

export async function submitWork(
  request: SubmitWorkRequest,
  options: { sessionID?: string | null } = {},
): Promise<SubmitWorkResponse> {
  const response = await fetch(
    factoryAPIURL(factorySessionScopedPath(SUBMIT_WORK_ENDPOINT, options.sessionID)),
    {
      body: JSON.stringify(request),
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    },
  );

  if (response.status === 201) {
    return (await response.json()) as SubmitWorkResponse;
  }

  throw await submitWorkErrorFromResponse(response);
}

export async function stageSubmitWorkFile(
  request: StageSubmitWorkFileRequest,
  options: { sessionID?: string | null } = {},
): Promise<StageSubmitWorkFileResponse> {
  const response = await fetch(
    factoryAPIURL(
      factorySessionScopedPath(STAGE_SUBMIT_WORK_FILE_ENDPOINT, options.sessionID),
    ),
    {
      body: JSON.stringify(request),
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    },
  );

  if (response.status === 201) {
    return (await response.json()) as StageSubmitWorkFileResponse;
  }

  throw await stageSubmitWorkFileErrorFromResponse(response);
}

async function submitWorkErrorFromResponse(response: Response): Promise<SubmitWorkAPIError> {
  const responseBody = await readAPIResponseBody(response);
  const errorResponse = extractAPIErrorPayload(responseBody);
  const message = normalizeSubmitWorkErrorMessage(errorResponse?.message);
  return new SubmitWorkAPIError({
    code: message ? normalizeSubmitWorkErrorCode(errorResponse?.code) : undefined,
    message: message ?? GENERIC_SUBMIT_WORK_ERROR_MESSAGE,
    status: response.status,
    statusText: response.statusText,
  });
}

function normalizeSubmitWorkErrorMessage(message: string | undefined): string | undefined {
  if (typeof message !== "string") {
    return undefined;
  }

  return message.length > 0 ? message : undefined;
}

function normalizeSubmitWorkErrorCode(
  code: string | undefined,
): ErrorResponse["code"] | undefined {
  switch (code) {
    case "BAD_REQUEST":
    case "INVALID_FACTORY_NAME":
    case "FACTORY_ALREADY_EXISTS":
    case "INVALID_FACTORY":
    case "FACTORY_NOT_IDLE":
    case "STALE_FACTORY_VERSION":
    case "NOT_FOUND":
    case "INTERNAL_ERROR":
      return code;
    default:
      return "INTERNAL_ERROR";
  }
}

export function isSubmitWorkAPIError(error: unknown): error is SubmitWorkAPIError {
  return error instanceof SubmitWorkAPIError;
}

async function stageSubmitWorkFileErrorFromResponse(
  response: Response,
): Promise<StageSubmitWorkFileAPIError> {
  const responseBody = await readAPIResponseBody(response);
  const errorResponse = extractAPIErrorPayload(responseBody);
  const message = normalizeSubmitWorkErrorMessage(errorResponse?.message);
  return new StageSubmitWorkFileAPIError({
    code: message ? normalizeSubmitWorkErrorCode(errorResponse?.code) : undefined,
    message: message ?? GENERIC_STAGE_SUBMIT_WORK_FILE_ERROR_MESSAGE,
    status: response.status,
    statusText: response.statusText,
  });
}

export function isStageSubmitWorkFileAPIError(
  error: unknown,
): error is StageSubmitWorkFileAPIError {
  return error instanceof StageSubmitWorkFileAPIError;
}
