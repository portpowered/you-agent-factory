import { useQuery } from "@tanstack/react-query";

import {
  type FactoryPreviewAPIError,
  type FactoryPreviewRequest,
  type FactoryPreviewResult,
  previewFactory,
} from "../../../api/factory-preview";

export function buildFactoryPreviewQueryKey(request: FactoryPreviewRequest) {
  return [
    "factory-preview",
    request.sourceKind,
    request.projectRoot ?? "",
    request.sourceValue ?? "",
    request.inlineSource ?? "",
    request.artifactRoot ?? "",
  ] as const;
}

/** @deprecated Use buildFactoryPreviewQueryKey. */
export const buildWorkflowPreviewQueryKey = buildFactoryPreviewQueryKey;

export function factoryPreviewQueryOptions(
  request: FactoryPreviewRequest | null,
  isEnabled = true,
) {
  return {
    queryKey: request
      ? buildFactoryPreviewQueryKey(request)
      : (["factory-preview", "missing-request"] as const),
    queryFn: () => {
      if (!request) {
        throw new Error("factory preview request is required");
      }
      return previewFactory(request);
    },
    enabled: isEnabled && request != null,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  } as const;
}

/** @deprecated Use factoryPreviewQueryOptions. */
export const workflowPreviewQueryOptions = factoryPreviewQueryOptions;

export function useFactoryPreview(
  request: FactoryPreviewRequest | null,
  isEnabled = true,
) {
  return useQuery<FactoryPreviewResult, FactoryPreviewAPIError>(
    factoryPreviewQueryOptions(request, isEnabled),
  );
}

/** @deprecated Use useFactoryPreview. */
export const useWorkflowPreview = useFactoryPreview;
