/**
 * Hook-level mocks for current-selection specs that previously used Vitest-hoisted
 * `vi.mock` against modules imported in the same file. Import before consumers.
 */
import { mock } from "bun:test";

import "./bun-current-factory-definition-public-mocks";

const PROMPT_TEMPLATE_CONTRACT_HOOK_MODULE =
  "../src/features/current-selection/hooks/useCurrentWorkstationPromptTemplateContract";
const PROMPT_TEMPLATE_VALIDATION_HOOK_MODULE =
  "../src/features/current-selection/hooks/useCurrentWorkstationPromptTemplateValidation";
const promptTemplateContractHookActual = await import(
  PROMPT_TEMPLATE_CONTRACT_HOOK_MODULE,
);
const promptTemplateValidationHookActual = await import(
  PROMPT_TEMPLATE_VALIDATION_HOOK_MODULE,
);

export const useCurrentWorkstationPromptTemplateContractMock = mock(() => ({
  data: undefined,
  error: null,
  isError: false,
  isFetching: false,
  isPending: true,
  isSuccess: false,
  status: "pending",
}));

export const useCurrentWorkstationPromptTemplateValidationMock = mock(() => ({
  data: undefined,
  error: null,
  isError: false,
  isFetching: false,
  isPending: true,
  isSuccess: false,
  status: "pending",
}));

mock.module(PROMPT_TEMPLATE_CONTRACT_HOOK_MODULE, () => ({
  ...promptTemplateContractHookActual,
  useCurrentWorkstationPromptTemplateContract:
    useCurrentWorkstationPromptTemplateContractMock,
}));

mock.module(PROMPT_TEMPLATE_VALIDATION_HOOK_MODULE, () => ({
  ...promptTemplateValidationHookActual,
  useCurrentWorkstationPromptTemplateValidation:
    useCurrentWorkstationPromptTemplateValidationMock,
}));
