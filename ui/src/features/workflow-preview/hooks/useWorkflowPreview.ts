import { useQuery } from "@tanstack/react-query";

import {
  type WorkflowPreviewAPIError,
  type WorkflowPreviewRequest,
  type WorkflowPreviewResult,
  previewWorkflow,
} from "../../../api/workflow-preview";

export function buildWorkflowPreviewQueryKey(request: WorkflowPreviewRequest) {
  return [
    "workflow-preview",
    request.sourceKind,
    request.projectRoot ?? "",
    request.sourceValue ?? "",
    request.inlineSource ?? "",
    request.artifactRoot ?? "",
  ] as const;
}

export function useWorkflowPreview(
  request: WorkflowPreviewRequest | null,
  isEnabled = true,
) {
  return useQuery<WorkflowPreviewResult, WorkflowPreviewAPIError>({
    queryKey: request
      ? buildWorkflowPreviewQueryKey(request)
      : (["workflow-preview", "missing-request"] as const),
    queryFn: () => {
      if (!request) {
        throw new Error("workflow preview request is required");
      }
      return previewWorkflow(request);
    },
    enabled: isEnabled && request != null,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}
