/**
 * Partial mocks for `api/current-factory-definition` in hook unit specs.
 */
import { mock } from "bun:test";

const CURRENT_FACTORY_API_MODULE =
  "../src/api/current-factory-definition";

const currentFactoryApiActual = await import(CURRENT_FACTORY_API_MODULE);

export const getCurrentFactoryDefinitionMock = mock(() => {
  throw new Error("getCurrentFactoryDefinitionMock not configured");
});
export const getCurrentFactoryDocumentMock = mock(() => {
  throw new Error("getCurrentFactoryDocumentMock not configured");
});
export const saveFactoryForSessionDocumentMock = mock(() => {
  throw new Error("saveFactoryForSessionDocumentMock not configured");
});

/** @deprecated use {@link saveFactoryForSessionDocumentMock} */
export const saveCurrentFactoryDocumentMock = saveFactoryForSessionDocumentMock;

mock.module(CURRENT_FACTORY_API_MODULE, () => ({
  ...currentFactoryApiActual,
  getCurrentFactoryDefinition: getCurrentFactoryDefinitionMock,
  getCurrentFactoryDocument: getCurrentFactoryDocumentMock,
  saveFactoryForSessionDocument: saveFactoryForSessionDocumentMock,
  saveCurrentFactoryDocument: saveFactoryForSessionDocumentMock,
}));
