import { useQuery } from "@tanstack/react-query";

import {
  type CurrentFactoryPromptTemplateAPIError,
  getCurrentFactoryWorkstationPromptTemplateContract,
  type PromptTemplateContract,
} from "../../../api/current-factory-prompt-template";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";

export function buildCurrentWorkstationPromptTemplateContractQueryKey(
  workstationName: string,
  sessionID: string | null | undefined,
) {
  return [
    "current-workstation-prompt-template-contract",
    sessionID ?? DEFAULT_FACTORY_SESSION_ID,
    workstationName,
  ] as const;
}

export function useCurrentWorkstationPromptTemplateContract(
  workstationName: string | undefined,
  isEnabled = true,
) {
  const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);

  return useQuery<PromptTemplateContract, CurrentFactoryPromptTemplateAPIError>(
    {
      queryKey: workstationName
        ? buildCurrentWorkstationPromptTemplateContractQueryKey(
            workstationName,
            sessionID,
          )
        : [
            "current-workstation-prompt-template-contract",
            sessionID ?? DEFAULT_FACTORY_SESSION_ID,
            "missing-workstation",
          ],
      queryFn: () => {
        if (!workstationName) {
          throw new Error("workstationName is required");
        }

        return getCurrentFactoryWorkstationPromptTemplateContract(
          workstationName,
          { sessionID },
        );
      },
      enabled: isEnabled && Boolean(workstationName),
      gcTime: 0,
      refetchOnWindowFocus: false,
      retry: false,
    },
  );
}
