import type { SessionFactoryAPIErrorCode } from "./errors";
import { sessionFactoryAPIErrorMessages } from "./messages";

export const sessionFactoryOperatorErrorMessages = {
  FACTORY_NOT_IDLE:
    "The current factory runtime is still active. Wait until it becomes idle before saving or switching factories.",
  INVALID_FACTORY:
    "The factory definition was rejected by the session factory API.",
  INVALID_FACTORY_NAME: "The factory name is not valid for this session.",
  STALE_FACTORY_VERSION:
    "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
} as const satisfies Partial<
  Record<SessionFactoryAPIErrorCode, string>
>;

export interface ResolveSessionFactoryAPIErrorMessageInput {
  apiMessage?: string;
  code: SessionFactoryAPIErrorCode;
  rejectedMessage: string;
  status?: number;
}

export function resolveSessionFactoryAPIErrorMessage(
  input: ResolveSessionFactoryAPIErrorMessageInput,
): string {
  const operatorMessage =
    sessionFactoryOperatorErrorMessages[
      input.code as keyof typeof sessionFactoryOperatorErrorMessages
    ];

  if (operatorMessage) {
    return operatorMessage;
  }

  switch (input.code) {
    case "NETWORK_ERROR":
      return sessionFactoryAPIErrorMessages.network;
    case "INTERNAL_ERROR":
      if (input.status !== undefined && input.status >= 500) {
        return input.rejectedMessage;
      }
      return input.apiMessage ?? input.rejectedMessage;
    default:
      return input.apiMessage ?? input.rejectedMessage;
  }
}
