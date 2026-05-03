import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";

type SubmitWorkRequest = components["schemas"]["SubmitWorkRequest"];
type SubmitWorkResponse = components["schemas"]["SubmitWorkResponse"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

const SUBMIT_WORK_ENDPOINT = "/work";
const GENERIC_SUBMIT_WORK_ERROR_MESSAGE =
  "Dashboard submission failed. Try again in a moment.";

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

export async function submitWork(request: SubmitWorkRequest): Promise<SubmitWorkResponse> {
  const response = await fetch(factoryAPIURL(SUBMIT_WORK_ENDPOINT), {
    body: JSON.stringify(request),
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  });

  if (response.status === 201) {
    return (await response.json()) as SubmitWorkResponse;
  }

  throw await submitWorkErrorFromResponse(response);
}

async function submitWorkErrorFromResponse(response: Response): Promise<SubmitWorkAPIError> {
  const errorResponse = await parseErrorResponse(response);
  return new SubmitWorkAPIError({
    code: errorResponse?.code,
    message: errorResponse?.message ?? GENERIC_SUBMIT_WORK_ERROR_MESSAGE,
    status: response.status,
    statusText: response.statusText,
  });
}

async function parseErrorResponse(response: Response): Promise<ErrorResponse | null> {
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return null;
  }

  try {
    const payload = (await response.json()) as Partial<ErrorResponse>;
    if (typeof payload.message !== "string" || payload.message.length === 0) {
      return null;
    }

    return {
      code: payload.code ?? "INTERNAL_ERROR",
      family: payload.family ?? "INTERNAL_SERVER_ERROR",
      message: payload.message,
    };
  } catch {
    return null;
  }
}

export function isSubmitWorkAPIError(error: unknown): error is SubmitWorkAPIError {
  return error instanceof SubmitWorkAPIError;
}

