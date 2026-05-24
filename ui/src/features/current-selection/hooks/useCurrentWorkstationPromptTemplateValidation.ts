import { useQuery } from "@tanstack/react-query";

import {
  type CurrentFactoryPromptTemplateAPIError,
  type PromptTemplateValidationResult,
  validateCurrentFactoryWorkstationPromptTemplate,
} from "../../../api/current-factory-prompt-template";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";

export function buildCurrentWorkstationPromptTemplateValidationQueryKey(
  workstationName: string,
  prompt: string,
  sessionID: string | null | undefined,
) {
  return [
    "current-workstation-prompt-template-validation",
    sessionID ?? DEFAULT_FACTORY_SESSION_ID,
    workstationName,
    prompt,
  ] as const;
}

export function useCurrentWorkstationPromptTemplateValidation(
  workstationName: string | undefined,
  prompt: string | undefined,
  isEnabled = true,
) {
  const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);

  return useQuery<
    PromptTemplateValidationResult,
    CurrentFactoryPromptTemplateAPIError
  >({
    queryKey:
      workstationName && typeof prompt === "string"
        ? buildCurrentWorkstationPromptTemplateValidationQueryKey(
            workstationName,
            prompt,
            sessionID,
          )
        : [
            "current-workstation-prompt-template-validation",
            sessionID ?? DEFAULT_FACTORY_SESSION_ID,
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
        { sessionID },
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
