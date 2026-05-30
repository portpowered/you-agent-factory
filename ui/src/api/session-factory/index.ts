/**
 * Session factory API family: factory GET/PUT plus session-scoped workstation
 * prompt-template contract and validation routes (same `/factory-sessions/...` prefix).
 */
export * from "./api";
export {
  normalizeSessionFactoryAPIErrorCode,
  SessionFactoryAPIError,
  type SessionFactoryAPIErrorCode,
  type SessionFactoryAPIErrorDetails,
} from "./errors";
export * from "./prompt-template";
