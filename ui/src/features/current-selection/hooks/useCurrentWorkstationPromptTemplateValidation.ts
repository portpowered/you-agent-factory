import { useQuery } from "@tanstack/react-query";

import {
  type CurrentFactoryPromptTemplateAPIError,
  type PromptTemplateValidationResult,
  validateCurrentFactoryWorkstationPromptTemplate,
} from "../../../api/current-factory-prompt-template";

export function buildCurrentWorkstationPromptTemplateValidationQueryKey(
  workstationName: string,
  prompt: string,
) {
  return [
    "current-workstation-prompt-template-validation",
    workstationName,
    prompt,
  ] as const;
}

export function useCurrentWorkstationPromptTemplateValidation(
  workstationName: string | undefined,
  prompt: string | undefined,
  isEnabled = true,
) {
  return useQuery<
    PromptTemplateValidationResult,
    CurrentFactoryPromptTemplateAPIError
  >({
    queryKey:
      workstationName && typeof prompt === "string"
        ? buildCurrentWorkstationPromptTemplateValidationQueryKey(
            workstationName,
            prompt,
          )
        : [
            "current-workstation-prompt-template-validation",
            "missing-workstation",
            "missing-prompt",
          ],
    queryFn: () => {
      if (!workstationName) {
        throw new Error("workstationName is required");
      }
      if (typeof prompt !== "string") {
        throw new Error("prompt is required");
      }

      return validateCurrentFactoryWorkstationPromptTemplate(
        workstationName,
        prompt,
      );
    },
    enabled:
      isEnabled &&
      Boolean(workstationName) &&
      typeof prompt === "string" &&
      prompt.trim().length > 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}
