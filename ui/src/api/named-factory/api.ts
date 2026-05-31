/**
 * @deprecated Import from `session-factory` instead. Removed in ui-api-module-cleanup-005.
 */
export {
  getCurrentFactory,
  type FactoryValue,
  type GetCurrentFactoryOptions,
} from "../session-factory/export";
export {
  SessionFactoryAPIError as NamedFactoryAPIError,
  type SessionFactoryAPIErrorCode as NamedFactoryAPIErrorCode,
  type SessionFactoryAPIErrorDetails as NamedFactoryAPIErrorDetails,
} from "../session-factory/errors";
