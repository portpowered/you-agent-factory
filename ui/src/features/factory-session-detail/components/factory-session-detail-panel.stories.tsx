import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import {
  awaitingReplaySessionID,
  buildAwaitingDurableSession,
  buildAwaitingReplayEventStream,
  buildSuccessfulDurableSession,
  buildSuccessfulReplayDispatchList,
  buildSuccessfulReplayEventStream,
  buildWarningDurableSession,
  buildWarningReplayDispatchList,
  buildWarningReplayEventStream,
  successfulReplaySessionID,
  unavailableReplaySessionID,
  warningReplaySessionID,
} from "../../../testing/factory-session-event-replay-fixtures";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const storySessionID = "session-beta";

export default {
  title: "you-agent-factory/Current Selection/Factory Session Detail Panel",
  component: FactorySessionDetailPanel,
};

function renderFactorySessionDetailPanel(sessionID: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return (
    <div style={{ maxWidth: "100%", width: "960px" }}>
      <QueryClientProvider client={queryClient}>
        <FactorySessionDetailPanel sessionID={sessionID} />
      </QueryClientProvider>
    </div>
  );
}

export const DispatchDrilldownStates = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}`,
          response: {
            body: {
              factoryDir: "/workspace/root/beta",
              folderPath: "/workspace/root",
              id: storySessionID,
              isDefault: false,
              project: "beta",
              runtime: {
                dispatches: [
                  {
                    dispatchKind: "JAVASCRIPT_AGENT",
                    id: "dispatch-success",
                    label: "Review child task",
                    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                    sessionId: storySessionID,
                    status: "COMPLETED",
                  },
                  {
                    dispatchKind: "JAVASCRIPT_VERIFY",
                    id: "dispatch-failed",
                    label: "Verify release manifest",
                    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                    sessionId: storySessionID,
                    status: "FAILED",
                  },
                  {
                    dispatchKind: "JAVASCRIPT_AGENT",
                    id: "dispatch-missing",
                    label: "Missing durable detail",
                    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                    sessionId: storySessionID,
                    status: "FAILED",
                  },
                  {
                    dispatchKind: "JAVASCRIPT_AGENT",
                    id: "dispatch-error",
                    label: "Errored durable detail",
                    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                    sessionId: storySessionID,
                    status: "FAILED",
                  },
                ],
                javascript: {
                  childDispatchCounts: {
                    completed: 1,
                    queued: 0,
                    running: 0,
                  },
                  phase: "review",
                  phases: ["review"],
                  scriptStatus: "IDLE",
                },
                lifecycle: {
                  startedAt: "2026-06-08T14:00:00Z",
                  updatedAt: "2026-06-08T14:05:00Z",
                },
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                progress: {
                  categories: {},
                  factoryState: "RUNNING",
                  inFlightCount: 0,
                  totalTokens: 0,
                },
                status: "IDLE",
                usage: { resources: [] },
              },
              target: { kind: "named", name: "beta" },
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/result`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/partial-result`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/dispatches/dispatch-success`,
          response: {
            body: {
              artifactIds: ["artifact-success-1"],
              attempt: 2,
              dispatchKind: "JAVASCRIPT_AGENT",
              id: "dispatch-success",
              javascript: {
                executionMode: "live",
                taskKind: "AGENT",
                taskLabel: "Review child task",
              },
              label: "Review child task",
              model: "gpt-5.5",
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              phase: "review",
              provider: "openai",
              providerSessionRefs: [
                {
                  id: "provider-session-1",
                  kind: "response_id",
                  provider: "openai",
                },
              ],
              relatedWorkIds: ["work-123"],
              runnerId: "runner-a",
              sessionId: storySessionID,
              status: "COMPLETED",
              statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
              usage: {
                costUsd: 0.21,
                durationMillis: 4400,
                inputTokens: 120,
                outputTokens: 80,
                retryCount: 1,
                totalTokens: 200,
              },
              warnings: [
                {
                  code: "DISPATCH_WARNING",
                  message: "Token budget was nearly exhausted.",
                },
              ],
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/dispatches/dispatch-failed`,
          response: {
            body: {
              artifactIds: ["artifact-failure-log"],
              dispatchKind: "JAVASCRIPT_VERIFY",
              failureDetail: {
                errorClass: "verification_error",
                message: "Expected release manifest checksum.",
                reason: "VERIFY_ASSERTION_FAILED",
              },
              id: "dispatch-failed",
              javascript: {
                executionMode: "live",
                taskKind: "VERIFY",
                taskLabel: "Verify release manifest",
              },
              label: "Verify release manifest",
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              relatedWorkIds: ["work-verify-1"],
              sessionId: storySessionID,
              status: "FAILED",
              statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/dispatches/dispatch-missing`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/dispatches/dispatch-error`,
          response: {
            body: {
              code: "INTERNAL_ERROR",
              message: "dispatch boom",
            },
            status: 500,
          },
        },
      ],
      sessionID: storySessionID,
    },
  },
  render: () => renderFactorySessionDetailPanel(storySessionID),
};

export const DurableReplayDisclosure = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}`,
          response: {
            body: buildSuccessfulDurableSession(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}/events`,
          response: {
            body: buildSuccessfulReplayEventStream(),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}/dispatches`,
          response: {
            body: buildSuccessfulReplayDispatchList(),
          },
        },
      ],
      sessionID: successfulReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText("Showing 5 Factory Events.");
    expect(canvas.getByText("Session completed")).toBeTruthy();
    expect(canvas.getByText("Dispatch status completed")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(successfulReplaySessionID),
};

export const DurableReplayDisclosureAwaitingApproval = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${awaitingReplaySessionID}`,
          response: {
            body: buildAwaitingDurableSession(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${awaitingReplaySessionID}/events`,
          response: {
            body: buildAwaitingReplayEventStream(),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
      ],
      sessionID: awaitingReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();

    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await user.click(trigger);
    await expect(canvas.findByText("Showing 2 Factory Events.")).resolves.toBeTruthy();
    await expect(canvas.getByText("Session result updated")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(awaitingReplaySessionID),
};

export const DurableReplayDisclosureWarning = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}`,
          response: {
            body: buildWarningDurableSession(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}/events`,
          response: {
            body: buildWarningReplayEventStream(),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}/dispatches`,
          response: {
            body: buildWarningReplayDispatchList(),
          },
        },
      ],
      sessionID: warningReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText("Checkpoint recorded");
    expect(canvas.getByText("Provider session timed out · Retry planned")).toBeTruthy();
    expect(canvas.getByText("Release verification failed.")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(warningReplaySessionID),
};

export const DurableReplayDisclosureUnavailable = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}`,
          response: {
            body: buildWarningDurableSession(unavailableReplaySessionID),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}/events`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}/dispatches`,
          response: {
            body: {
              dispatches: [],
              sessionId: unavailableReplaySessionID,
            },
          },
        },
      ],
      sessionID: unavailableReplaySessionID,
    },
  },
  render: () => renderFactorySessionDetailPanel(unavailableReplaySessionID),
};
