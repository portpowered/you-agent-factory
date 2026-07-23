import { describe, expect, it } from "vitest";

import type { FactorySessionLifecycleControlResponse } from "../../../../api/factory-sessions";
import { getFactorySessionDetailMessages } from "../../messages/factory-session-detail";
import {
  type LifecycleControlFeedbackState,
  resolveFactorySessionLifecycleFeedbackDisplay,
} from "./factory-session-lifecycle-feedback";

describe("factory session lifecycle feedback", () => {
  const messages = getFactorySessionDetailMessages("en");

  it("maps accepted responses to success feedback with refreshed status detail", () => {
    const display = resolveFactorySessionLifecycleFeedbackDisplay(
      resolvedFeedback({
        operation: "PAUSE",
        outcome: "ACCEPTED",
        retryDispatchId: "dispatch-retry-1",
        sessionId: "dur-sess-1",
        status: "PAUSED",
      }),
      messages,
    );

    expect(display).toEqual({
      detail:
        "Current durable status: Paused. Retry dispatch: dispatch-retry-1.",
      outcomeLabel: "Accepted",
      role: "status",
      title: "Pause accepted",
      tone: "success",
    });
  });

  it("maps no-op, invalid-state, terminal-session, and conflict outcomes to explicit user-visible treatment", () => {
    expect(
      resolveFactorySessionLifecycleFeedbackDisplay(
        resolvedFeedback({
          detail: "Session is already paused.",
          operation: "PAUSE",
          outcome: "NO_OP",
          sessionId: "dur-sess-1",
          status: "PAUSED",
        }),
        messages,
      ),
    ).toMatchObject({
      outcomeLabel: "No-op",
      role: "status",
      title: "Pause was already satisfied.",
      tone: "info",
    });

    expect(
      resolveFactorySessionLifecycleFeedbackDisplay(
        resolvedFeedback({
          operation: "RESUME",
          outcome: "INVALID_STATE",
          sessionId: "dur-sess-1",
          status: "RUNNING",
        }),
        messages,
      ),
    ).toMatchObject({
      outcomeLabel: "Invalid state",
      role: "alert",
      title: "Resume is not available in the current session state.",
      tone: "warning",
    });

    expect(
      resolveFactorySessionLifecycleFeedbackDisplay(
        resolvedFeedback({
          operation: "TERMINATE",
          outcome: "TERMINAL_SESSION",
          sessionId: "dur-sess-1",
          status: "SUCCEEDED",
        }),
        messages,
      ),
    ).toMatchObject({
      outcomeLabel: "Terminal session",
      role: "alert",
      title:
        "Terminate is unavailable because this Factory Session is already terminal.",
      tone: "warning",
    });

    expect(
      resolveFactorySessionLifecycleFeedbackDisplay(
        resolvedFeedback({
          detail: "Another resume is still being applied.",
          operation: "RESUME",
          outcome: "CONFLICT",
          sessionId: "dur-sess-1",
          status: "RESUMING",
        }),
        messages,
      ),
    ).toMatchObject({
      outcomeLabel: "Conflict",
      role: "alert",
      title: "Resume is blocked by another lifecycle change.",
      tone: "warning",
    });
  });

  it("maps transport errors to danger feedback", () => {
    const display = resolveFactorySessionLifecycleFeedbackDisplay(
      {
        actionID: "terminate",
        kind: "transport-error",
        message: "The dashboard could not reach the factory sessions API.",
      },
      messages,
    );

    expect(display).toEqual({
      detail: "The dashboard could not reach the factory sessions API.",
      outcomeLabel: "Request failed",
      role: "alert",
      title: "Terminate could not be submitted.",
      tone: "danger",
    });
  });
});

function resolvedFeedback(
  response: FactorySessionLifecycleControlResponse,
): LifecycleControlFeedbackState {
  return {
    actionID: actionIDFromOperation(response.operation),
    kind: "resolved",
    response,
  };
}

function actionIDFromOperation(
  operation: FactorySessionLifecycleControlResponse["operation"],
): LifecycleControlFeedbackState["actionID"] {
  switch (operation) {
    case "APPROVE":
      return "approve";
    case "CANCEL":
      return "cancel";
    case "PAUSE":
      return "pause";
    case "RESUME":
      return "resume";
    case "RETRY_DISPATCH":
      return "retry-dispatch";
    case "TERMINATE":
      return "terminate";
    default:
      return "pause";
  }
}
