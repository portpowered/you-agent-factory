import type { components } from "../generated/openapi";

type FactoryValidationTarget = components["schemas"]["FactoryValidationTarget"];

export type SessionFactoryAPIErrorCode =
  | "BAD_REQUEST"
  | "FACTORY_ALREADY_EXISTS"
  | "FACTORY_NOT_IDLE"
  | "INTERNAL_ERROR"
  | "INVALID_FACTORY"
  | "INVALID_FACTORY_NAME"
  | "NETWORK_ERROR"
  | "NOT_FOUND"
  | "STALE_FACTORY_VERSION";

export interface SessionFactoryAPIErrorDetails {
  cause?: unknown;
  code: SessionFactoryAPIErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
  targets?: FactoryValidationTarget[];
}

export class SessionFactoryAPIError extends Error {
  public readonly cause?: unknown;
  public readonly code: SessionFactoryAPIErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;
  public readonly targets?: FactoryValidationTarget[];

  public constructor(message: string, details: SessionFactoryAPIErrorDetails) {
    super(message);
    this.name = "SessionFactoryAPIError";
    this.cause = details.cause;
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
    this.targets = details.targets;
  }
}

export function normalizeSessionFactoryAPIErrorCode(
  code: string | undefined,
): SessionFactoryAPIErrorCode {
  switch (code) {
    case "BAD_REQUEST":
    case "FACTORY_ALREADY_EXISTS":
    case "FACTORY_NOT_IDLE":
    case "INVALID_FACTORY":
    case "INVALID_FACTORY_NAME":
    case "NOT_FOUND":
    case "STALE_FACTORY_VERSION":
      return code;
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "INTERNAL_ERROR";
  }
}
