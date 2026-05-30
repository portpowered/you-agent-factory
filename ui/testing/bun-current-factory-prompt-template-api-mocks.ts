/**
 * Partial API mocks for current-factory prompt-template hook specs.
 * Import before modules that consume `../../../api/current-factory-prompt-template`.
 */
import { mock } from "bun:test";

const PROMPT_TEMPLATE_API_MODULE =
  "../src/api/current-factory-prompt-template";

const promptTemplateApiActual = await import(PROMPT_TEMPLATE_API_MODULE);

export const validateCurrentFactoryWorkstationPromptTemplateMock = mock(() => {
  throw new Error(
    "validateCurrentFactoryWorkstationPromptTemplateMock not configured",
  );
});

export const getCurrentFactoryWorkstationPromptTemplateContractMock = mock(
  () => {
    throw new Error(
      "getCurrentFactoryWorkstationPromptTemplateContractMock not configured",
    );
  },
);

mock.module(PROMPT_TEMPLATE_API_MODULE, () => ({
  ...promptTemplateApiActual,
  validateCurrentFactoryWorkstationPromptTemplate:
    validateCurrentFactoryWorkstationPromptTemplateMock,
  getCurrentFactoryWorkstationPromptTemplateContract:
    getCurrentFactoryWorkstationPromptTemplateContractMock,
}));
