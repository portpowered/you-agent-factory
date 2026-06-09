import { factoryAPIURL } from "../baseUrl";
import type { components } from "../generated/openapi";
import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "../transport";
import { workflowPreviewAPIErrorMessages } from "./messages";

export type WorkflowPreviewRequest =
  components["schemas"]["WorkflowPreviewRequest"];
export type WorkflowPreviewResult =
  components["schemas"]["WorkflowPreviewResult"];
export type WorkflowDiagnostic = components["schemas"]["WorkflowDiagnostic"];

export type WorkflowPreviewAPIErrorCode =
  | "BAD_REQUEST"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export interface WorkflowPreviewAPIErrorDetails {
  code: WorkflowPreviewAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export interface WorkflowPreviewOptions {
  fetch?: typeof globalThis.fetch;
  signal?: AbortSignal;
}

export class WorkflowPreviewAPIError extends Error {
  public readonly code: WorkflowPreviewAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: WorkflowPreviewAPIErrorDetails,
  ) {
    super(message);
    this.name = "WorkflowPreviewAPIError";
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

const WORKFLOW_PREVIEW_ENDPOINT = "/workflow-previews";

export async function previewWorkflow(
  request: WorkflowPreviewRequest,
  options: WorkflowPreviewOptions = {},
): Promise<WorkflowPreviewResult> {
  const fetchImplementation = options.fetch ?? globalThis.fetch;

  if (typeof fetchImplementation !== "function") {
    throw new WorkflowPreviewAPIError(
      workflowPreviewAPIErrorMessages.emptyEnvironment,
      { code: "NETWORK_ERROR" },
    );
  }

  let response: Response;
  try {
    response = await fetchImplementation(
      factoryAPIURL(WORKFLOW_PREVIEW_ENDPOINT),
      {
        body: JSON.stringify(request),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
        signal: options.signal,
      },
    );
  } catch (error) {
    throw new WorkflowPreviewAPIError(
      workflowPreviewAPIErrorMessages.network,
      {
        code: "NETWORK_ERROR",
        responseBody: error,
      },
    );
  }

  const responseBody = await readAPIResponseBody(response);
  if (!response.ok) {
    throw new WorkflowPreviewAPIError(
      resolveWorkflowPreviewErrorMessage(responseBody),
      {
        code: resolveWorkflowPreviewErrorCode(response.status),
        responseBody,
        status: response.status,
        statusText: response.statusText,
      },
    );
  }

  if (!isWorkflowPreviewResult(responseBody)) {
    throw new WorkflowPreviewAPIError(
      workflowPreviewAPIErrorMessages.invalidResponse,
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

function resolveWorkflowPreviewErrorCode(
  status: number,
): WorkflowPreviewAPIErrorCode {
  if (status === 400) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return "BAD_REQUEST";
  }

  // hardcoded-ui-copy-exception: non-product-diagnostic
  return "INTERNAL_ERROR";
}

function resolveWorkflowPreviewErrorMessage(responseBody: unknown): string {
  const payload = extractAPIErrorPayload(responseBody);
  if (
    payload &&
    typeof payload.message === "string" &&
    payload.message.trim().length > 0
  ) {
    return payload.message;
  }

  return workflowPreviewAPIErrorMessages.rejectedRequest;
}

function isWorkflowPreviewResult(
  value: unknown,
): value is WorkflowPreviewResult {
  if (!isAPIRecord(value)) {
    return false;
  }

  return (
    typeof value.valid === "boolean" &&
    isAPIRecord(value.sourceResolution) &&
    Array.isArray(value.sourceValidationIssues) &&
    isAPIRecord(value.policyPreview) &&
    isAPIRecord(value.resultConstraints)
  );
}
