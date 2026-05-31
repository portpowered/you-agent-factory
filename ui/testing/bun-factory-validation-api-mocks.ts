/**
 * Partial mocks for `api/factory-validation` in factory-graph-editor hook specs.
 */
import { mock } from "bun:test";

const FACTORY_VALIDATION_API_MODULE = "../src/api/factory-validation";

const factoryValidationApiActual = await import(FACTORY_VALIDATION_API_MODULE);

export const validateFactoryDefinitionMock = mock(() => {
  throw new Error("validateFactoryDefinitionMock not configured");
});

mock.module(FACTORY_VALIDATION_API_MODULE, () => ({
  ...factoryValidationApiActual,
  validateFactoryDefinition: validateFactoryDefinitionMock,
}));
