/**
 * Partial API mocks for named-factory export hook specs.
 * Import before modules that consume `../../../api/named-factory`.
 */
import { mock } from "bun:test";

const NAMED_FACTORY_API_MODULE = "../src/api/named-factory";

const namedFactoryApiActual = await import(NAMED_FACTORY_API_MODULE);

export const getCurrentFactoryMock = mock(() => {
  throw new Error("getCurrentFactoryMock not configured");
});

mock.module(NAMED_FACTORY_API_MODULE, () => ({
  ...namedFactoryApiActual,
  getCurrentFactory: getCurrentFactoryMock,
}));
