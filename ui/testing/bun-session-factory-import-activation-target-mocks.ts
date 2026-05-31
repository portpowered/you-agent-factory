/**
 * Partial mocks for session-factory import activation target hook specs.
 */
import { mock } from "bun:test";

const SESSION_FACTORY_API_MODULE = "../src/api/session-factory";

const sessionFactoryApiActual = await import(SESSION_FACTORY_API_MODULE);

export const getSessionFactoryMock = mock(() => {
  throw new Error("getSessionFactoryMock not configured");
});
export const discoverSessionNamedFactoryNamesMock = mock(() => {
  throw new Error("discoverSessionNamedFactoryNamesMock not configured");
});

mock.module(SESSION_FACTORY_API_MODULE, () => ({
  ...sessionFactoryApiActual,
  getSessionFactory: getSessionFactoryMock,
  discoverSessionNamedFactoryNames: discoverSessionNamedFactoryNamesMock,
}));
