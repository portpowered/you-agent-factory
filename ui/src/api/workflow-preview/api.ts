import type {
  FactoryPreviewAPIError,
  FactoryPreviewRequest,
  FactoryPreviewResult,
} from "../factory-preview";
import { previewFactory } from "../factory-preview";

/** @deprecated Use FactoryPreviewRequest from `ui/src/api/factory-preview`. */
export type WorkflowPreviewRequest = FactoryPreviewRequest;
/** @deprecated Use FactoryPreviewResult from `ui/src/api/factory-preview`. */
export type WorkflowPreviewResult = FactoryPreviewResult;

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
