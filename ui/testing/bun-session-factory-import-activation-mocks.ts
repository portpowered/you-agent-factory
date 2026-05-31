/**
 * Partial mocks for session-factory import activation hook specs.
 */
import { mock } from "bun:test";

const SESSION_FACTORY_API_MODULE = "../src/api/session-factory";

const sessionFactoryApiActual = await import(SESSION_FACTORY_API_MODULE);

export const activateImportedFactoryForSessionMock = mock(
  sessionFactoryApiActual.activateImportedFactoryForSession,
);

mock.module(SESSION_FACTORY_API_MODULE, () => ({
  ...sessionFactoryApiActual,
  activateImportedFactoryForSession: activateImportedFactoryForSessionMock,
}));
