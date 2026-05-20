import { useQuery } from "@tanstack/react-query";

import {
  type CurrentFactoryPromptTemplateAPIError,
  getCurrentFactoryWorkstationPromptTemplateContract,
  type PromptTemplateContract,
} from "../../../api/current-factory-prompt-template";

export function buildCurrentWorkstationPromptTemplateContractQueryKey(
  workstationName: string,
) {
  return [
    "current-workstation-prompt-template-contract",
    workstationName,
  ] as const;
}

export function useCurrentWorkstationPromptTemplateContract(
  workstationName: string | undefined,
  isEnabled = true,
) {
  return useQuery<PromptTemplateContract, CurrentFactoryPromptTemplateAPIError>(
    {
      queryKey: workstationName
        ? buildCurrentWorkstationPromptTemplateContractQueryKey(workstationName)
        : [
            "current-workstation-prompt-template-contract",
            "missing-workstation",
          ],
      queryFn: () => {
        if (!workstationName) {
          throw new Error("workstationName is required");
        }

        return getCurrentFactoryWorkstationPromptTemplateContract(
          workstationName,
        );
      },
      enabled: isEnabled && Boolean(workstationName),
      gcTime: 0,
      refetchOnWindowFocus: false,
      retry: false,
    },
  );
}
