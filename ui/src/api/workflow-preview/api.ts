import type {
  FactoryPreviewAPIError,
  FactoryPreviewDiagnostic,
  FactoryPreviewRequest,
  FactoryPreviewResult,
} from "../factory-preview";
import { previewFactory } from "../factory-preview";

/** @deprecated Use FactoryPreviewRequest from `ui/src/api/factory-preview`. */
export type WorkflowPreviewRequest = FactoryPreviewRequest;
/** @deprecated Use FactoryPreviewResult from `ui/src/api/factory-preview`. */
export type WorkflowPreviewResult = FactoryPreviewResult;
/** @deprecated Use FactoryPreviewDiagnostic from `ui/src/api/factory-preview`. */
export type WorkflowDiagnostic = FactoryPreviewDiagnostic;

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

/** @deprecated Use FactoryPreviewAPIError from `ui/src/api/factory-preview`. */
export { FactoryPreviewAPIError as WorkflowPreviewAPIError } from "../factory-preview";

/** @deprecated Use previewFactory from `ui/src/api/factory-preview`. */
export async function previewWorkflow(
  request: WorkflowPreviewRequest,
  options: WorkflowPreviewOptions = {},
): Promise<WorkflowPreviewResult> {
  return previewFactory(request, options);
}
