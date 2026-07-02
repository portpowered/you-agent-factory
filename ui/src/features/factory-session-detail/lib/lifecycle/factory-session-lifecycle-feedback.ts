import type { FactorySessionLifecycleControlResponse } from "../../../../api/factory-sessions";
import type { FactorySessionDetailMessages } from "../../messages/factory-session-detail";
import type { FactorySessionLifecycleActionID } from "../factory-session-lifecycle-controls";

type LifecycleFeedbackTone = "danger" | "info" | "success" | "warning";

export type LifecycleControlFeedbackState =
  | {
      actionID: FactorySessionLifecycleActionID;
      kind: "resolved";
      response: FactorySessionLifecycleControlResponse;
    }
  | {
      actionID: FactorySessionLifecycleActionID;
      kind: "transport-error";
      message: string;
    };

export interface FactorySessionLifecycleFeedbackDisplay {
  detail?: string;
  outcomeLabel: string;
  role: "alert" | "status";
  title: string;
  tone: LifecycleFeedbackTone;
}

export function getFactorySessionLifecycleActionLabel(
  action: FactorySessionLifecycleActionID,
  messages: FactorySessionDetailMessages,
): string {
  switch (action) {
    case "approve":
      return messages.lifecycleActionApproveLabel;
    case "cancel":
      return messages.lifecycleActionCancelLabel;
    case "pause":
      return messages.lifecycleActionPauseLabel;
    case "resume":
      return messages.lifecycleActionResumeLabel;
    case "retry-dispatch":
      return messages.lifecycleActionRetryDispatchLabel;
    case "terminate":
      return messages.lifecycleActionTerminateLabel;
  }
}

export function resolveFactorySessionLifecycleFeedbackDisplay(
  feedback: LifecycleControlFeedbackState,
  messages: FactorySessionDetailMessages,
): FactorySessionLifecycleFeedbackDisplay {
  const actionLabel = getFactorySessionLifecycleActionLabel(
    feedback.actionID,
    messages,
  );

  if (feedback.kind === "transport-error") {
    return {
      detail: feedback.message,
      outcomeLabel: messages.lifecycleOutcomeTransportErrorLabel,
      role: "alert",
      title: messages.lifecycleOutcomeTransportErrorTitle(actionLabel),
      tone: "danger",
    };
  }

  const currentStatusDetail = messages.lifecycleOutcomeCurrentStatusDetail(
    messages.durableLifecycleStatusLabels[feedback.response.status],
  );
  const detailParts = [
    feedback.response.detail,
    currentStatusDetail,
    feedback.response.retryDispatchId
      ? messages.lifecycleOutcomeRetryDispatchDetail(
          feedback.response.retryDispatchId,
        )
      : undefined,
  ].filter((part): part is string => typeof part === "string" && part.length > 0);

  switch (feedback.response.outcome) {
    case "ACCEPTED":
      return {
        detail: detailParts.join(" "),
        outcomeLabel: messages.lifecycleOutcomeAcceptedLabel,
        role: "status",
        title: messages.lifecycleOutcomeAcceptedTitle(actionLabel),
        tone: "success",
      };
    case "NO_OP":
      return {
        detail: detailParts.join(" "),
        outcomeLabel: messages.lifecycleOutcomeNoOpLabel,
        role: "status",
        title: messages.lifecycleOutcomeNoOpTitle(actionLabel),
        tone: "info",
      };
    case "INVALID_STATE":
      return {
        detail: detailParts.join(" "),
        outcomeLabel: messages.lifecycleOutcomeInvalidStateLabel,
        role: "alert",
        title: messages.lifecycleOutcomeInvalidStateTitle(actionLabel),
        tone: "warning",
      };
    case "TERMINAL_SESSION":
      return {
        detail: detailParts.join(" "),
        outcomeLabel: messages.lifecycleOutcomeTerminalSessionLabel,
        role: "alert",
        title: messages.lifecycleOutcomeTerminalSessionTitle(actionLabel),
        tone: "warning",
      };
    case "CONFLICT":
      return {
        detail: detailParts.join(" "),
        outcomeLabel: messages.lifecycleOutcomeConflictLabel,
        role: "alert",
        title: messages.lifecycleOutcomeConflictTitle(actionLabel),
        tone: "warning",
      };
  }
}
