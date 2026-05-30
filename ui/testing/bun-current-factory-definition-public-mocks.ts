/**
 * Partial mocks for `current-factory-definition/public` in current-selection specs.
 */
import { mock } from "bun:test";

const FACTORY_DEFINITION_PUBLIC_MODULE =
  "../src/features/current-factory-definition/public";

const factoryDefinitionPublicActual = await import(
  FACTORY_DEFINITION_PUBLIC_MODULE,
);

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

export const useSaveCurrentFactoryMock = mock(() => ({
  mutate: () => undefined,
  mutateAsync: async () => undefined,
  status: "idle",
}));

mock.module(FACTORY_DEFINITION_PUBLIC_MODULE, () => ({
  ...factoryDefinitionPublicActual,
  useCurrentFactoryDocument: useCurrentFactoryDocumentMock,
  useCurrentFactoryDefinition: useCurrentFactoryDefinitionMock,
  useSaveCurrentFactory: useSaveCurrentFactoryMock,
}));
