/**
 * Partial mocks for current-factory-definition hooks in current-selection specs.
 */
import { mock } from "bun:test";

const FACTORY_DEFINITION_HOOKS_MODULE =
  "../src/features/current-factory-definition/hooks/useCurrentFactoryDefinition";

const FACTORY_DOCUMENT_SAVE_MODULE =
  "../src/features/current-factory-definition/hooks/useFactoryDocumentSave";

const factoryDefinitionHooksActual = await import(
  FACTORY_DEFINITION_HOOKS_MODULE,
);
const factoryDocumentSaveActual = await import(FACTORY_DOCUMENT_SAVE_MODULE);

function idleQueryResult() {
  return {
    data: undefined,
    error: null,
    isError: false,
    isFetching: false,
    isPending: true,
    isSuccess: false,
    status: "pending",
  };
}

export const useCurrentFactoryDocumentMock = mock(() => idleQueryResult());

export const useCurrentFactoryDefinitionMock = mock(() => idleQueryResult());

export const useFactoryDocumentSaveMock = mock(() => ({
  error: null,
  isPending: false,
  reset: () => undefined,
  save: () => undefined,
  saveAsync: async () => undefined,
}));

/** @deprecated Use `useFactoryDocumentSaveMock` — kept for existing Bun spec imports. */
export const useSaveCurrentFactoryMock = useFactoryDocumentSaveMock;

const factoryDefinitionHookMocks = {
  ...factoryDefinitionHooksActual,
  useCurrentFactoryDocument: useCurrentFactoryDocumentMock,
  useCurrentFactoryDefinition: useCurrentFactoryDefinitionMock,
};

mock.module(FACTORY_DEFINITION_HOOKS_MODULE, () => factoryDefinitionHookMocks);

mock.module(FACTORY_DOCUMENT_SAVE_MODULE, () => ({
  ...factoryDocumentSaveActual,
  useFactoryDocumentSave: useFactoryDocumentSaveMock,
}));
