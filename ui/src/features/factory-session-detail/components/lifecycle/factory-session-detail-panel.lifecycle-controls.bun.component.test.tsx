import { describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";

import type { FactorySessionLifecycleControlResponse } from "../../../../api/factory-sessions";
import {
  FactoryOrchestratorKind,
  FactorySessionJavaScriptScriptStatus,
  FactorySessionStatus,
} from "../../../../api/generated/openapi";
import type {
  FactorySessionDetailData,
  FactorySessionDetailViewState,
} from "../../hooks/use-factory-session-detail";
import type { FactorySessionLifecycleControl } from "../../hooks/use-factory-session-lifecycle-control";
import type { FactorySessionLifecycleActionID } from "../../lib/factory-session-lifecycle-controls";
import { resolveFactorySessionLifecycleActionAvailability } from "../../lib/factory-session-lifecycle-controls";
import type { LifecycleControlFeedbackState } from "../../lib/lifecycle/factory-session-lifecycle-feedback";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import { LifecycleActionSection } from "./lifecycle-action-section";

const SESSION_ID = "dur-sess-lifecycle-contract-001";

const runningDetailState: FactorySessionDetailViewState = {
  data: buildRunningDetailData(),
  status: "success",
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: lifecycle controller outcomes share one focused contract harness.
describe("Factory Session lifecycle controller contract", () => {
  it("submits the selected action through an injected controller from keyboard activation", async () => {
    const submittedActions: FactorySessionLifecycleActionID[] = [];
    const lifecycleControl = createLifecycleControl({
      submitLifecycleAction: async (actionID) => {
        submittedActions.push(actionID);
      },
    });

    renderPanel(lifecycleControl);

    const user = userEvent.setup();
    const pauseButton = screen.getByRole("button", { name: "Pause" });
    pauseButton.focus();
    await user.keyboard("{Enter}");

    expect(submittedActions).toEqual(["pause"]);
  });

  it("renders pending mutation state with sibling actions disabled", () => {
    renderPanel(
      createLifecycleControl({
        pendingActionID: "pause",
      }),
    );

    expect(screen.getByRole("button", { name: "Pause" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(screen.getByRole("button", { name: "Pause" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Terminate" })).toBeDisabled();
  });

  it("renders an injected accepted outcome without losing the session controls", () => {
    function AcceptedOutcomeHarness() {
      const [state, setState] = useState<
        Pick<FactorySessionLifecycleControl, "feedback" | "pendingActionID">
      >({
        feedback: null,
        pendingActionID: null,
      });

      return (
        <PanelHarness
          lifecycleControl={{
            ...state,
            submitLifecycleAction: async (actionID) => {
              setState({ feedback: null, pendingActionID: actionID });
              await Promise.resolve();
              setState({
                feedback: resolvedFeedback(actionID),
                pendingActionID: null,
              });
            },
          }}
        />
      );
    }

    render(<AcceptedOutcomeHarness />);
    fireEvent.click(screen.getByRole("button", { name: "Pause" }));

    return waitFor(() => {
      expect(screen.getByText("Accepted")).toBeTruthy();
      expect(screen.getByText("Pause accepted")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
    });
  });

  it("renders injected transport failure feedback while keeping retryable actions available", () => {
    renderPanel(
      createLifecycleControl({
        feedback: {
          actionID: "pause",
          kind: "transport-error",
          message: "Lifecycle control service unavailable.",
        },
      }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Lifecycle control service unavailable.",
    );
    expect(screen.getByText("Pause could not be submitted.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Pause" })).toBeEnabled();
  });

  it("submits retry and interrupt intents from selected dispatch controller inputs", () => {
    const submittedActions: FactorySessionLifecycleActionID[] = [];
    const lifecycleControl = createLifecycleControl({
      submitLifecycleAction: async (actionID) => {
        submittedActions.push(actionID);
      },
    });
    const detailData =
      runningDetailState.status === "success"
        ? runningDetailState.data
        : undefined;
    const failedDispatch = detailData?.dispatches?.find(
      (dispatch) => dispatch.status === "FAILED",
    );
    const runningDispatch = detailData?.dispatches?.find(
      (dispatch) => dispatch.status === "RUNNING",
    );

    if (!failedDispatch || !runningDispatch) {
      throw new Error("expected selected lifecycle dispatch fixtures");
    }

    const { rerender } = render(
      <LifecycleActionSection
        availability={resolveFactorySessionLifecycleActionAvailability({
          dispatches: [failedDispatch],
          durableLifecycleStatus: "FAILED",
          isDurableSession: true,
          selectedDispatchID: failedDispatch.id,
        })}
        feedback={null}
        onAction={lifecycleControl.submitLifecycleAction}
        pendingActionID={null}
      />,
    );
    expect(screen.getByText("Selected dispatch: dispatch-failed")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry dispatch" }));

    rerender(
      <LifecycleActionSection
        availability={resolveFactorySessionLifecycleActionAvailability({
          dispatches: [runningDispatch],
          durableLifecycleStatus: "RUNNING",
          isDurableSession: true,
          selectedDispatchID: runningDispatch.id,
        })}
        feedback={null}
        onAction={lifecycleControl.submitLifecycleAction}
        pendingActionID={null}
      />,
    );
    expect(
      screen.getByText("Selected dispatch: dispatch-running"),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Interrupt dispatch" }));

    expect(submittedActions).toEqual(["retry-dispatch", "interrupt-dispatch"]);
  });
});

function PanelHarness({
  lifecycleControl,
}: {
  lifecycleControl: FactorySessionLifecycleControl;
}) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <FactorySessionDetailPanel
        detailState={runningDetailState}
        lifecycleControl={lifecycleControl}
        sessionID={SESSION_ID}
      />
    </QueryClientProvider>
  );
}

function renderPanel(lifecycleControl: FactorySessionLifecycleControl) {
  return render(<PanelHarness lifecycleControl={lifecycleControl} />);
}

function createLifecycleControl(
  overrides: Partial<FactorySessionLifecycleControl> = {},
): FactorySessionLifecycleControl {
  return {
    feedback: null,
    pendingActionID: null,
    submitLifecycleAction: async () => undefined,
    ...overrides,
  };
}

function resolvedFeedback(
  actionID: FactorySessionLifecycleActionID,
): LifecycleControlFeedbackState {
  const response: FactorySessionLifecycleControlResponse = {
    detail: "Pause request was queued.",
    operation: "PAUSE",
    outcome: "ACCEPTED",
    sessionId: SESSION_ID,
    status: "PAUSED",
  };

  return { actionID, kind: "resolved", response };
}

function buildRunningDetailData(): FactorySessionDetailData {
  return {
    dispatches: [
      {
        dispatchKind: "JAVASCRIPT_AGENT",
        id: "dispatch-running",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        sessionId: SESSION_ID,
        status: "RUNNING",
      },
      {
        dispatchKind: "JAVASCRIPT_VERIFY",
        id: "dispatch-failed",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        sessionId: SESSION_ID,
        status: "FAILED",
      },
    ],
    durableLifecycleStatus: "RUNNING",
    session: {
      factoryDir: "/workspace/root/review",
      folderPath: "/workspace/root",
      id: SESSION_ID,
      isDefault: false,
      project: "review",
      runtime: {
        javascript: {
          childDispatchCounts: {
            completed: 0,
            queued: 0,
            running: 1,
          },
          phase: "review",
          phases: ["review"],
          scriptStatus: FactorySessionJavaScriptScriptStatus.RUNNING,
        },
        lifecycle: {
          startedAt: "2026-08-11T18:00:00Z",
          updatedAt: "2026-08-11T18:01:00Z",
        },
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        progress: {
          categories: {
            failed: 1,
            initial: 0,
            processing: 1,
            terminal: 0,
          },
          factoryState: "RUNNING",
          inFlightCount: 1,
          totalTokens: 0,
        },
        status: FactorySessionStatus.ACTIVE,
        usage: { resources: [] },
      },
      target: { kind: "named", name: "review" },
    },
  };
}
