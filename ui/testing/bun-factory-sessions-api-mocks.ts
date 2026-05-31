/**
 * Partial mocks for `api/factory-sessions` in dashboard session tab specs.
 */
import { mock } from "bun:test";

const FACTORY_SESSIONS_API_MODULE = "../src/api/factory-sessions";

const factorySessionsApiActual = await import(FACTORY_SESSIONS_API_MODULE);

export const listFactorySessionsMock = mock(() => {
  throw new Error("listFactorySessionsMock not configured");
});
export const openFactorySessionMock = mock(() => {
  throw new Error("openFactorySessionMock not configured");
});
export const closeFactorySessionMock = mock(() => {
  throw new Error("closeFactorySessionMock not configured");
});

mock.module(FACTORY_SESSIONS_API_MODULE, () => ({
  ...factorySessionsApiActual,
  listFactorySessions: listFactorySessionsMock,
  openFactorySession: openFactorySessionMock,
  closeFactorySession: closeFactorySessionMock,
}));
