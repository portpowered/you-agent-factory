import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "../components/factory-session-detail-panel";

const storySessionID = "session-beta";

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
  tags: ["test"],
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
                    javascript: {
                      executionMode: "live",
                      taskKind: "AGENT",
                      taskLabel: "Review child task",
                    },
                    label: "Review child task",
                    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                    providerSessionRefs: [
                      {
                        id: "provider-session-1",
                        kind: "session_id",
                        provider: "codex",
                      },
                    ],
                    sessionId: storySessionID,
                    status: "COMPLETED",
                  },
                  {
                    dispatchKind: "JAVASCRIPT_VERIFY",
                    id: "dispatch-failed",
                    label: "Verify release manifest",
                    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                    providerSessionRefs: [
                      {
                        id: "provider-session-verify-1",
                        kind: "session_id",
                        provider: "codex",
                      },
                    ],
                    sessionId: storySessionID,
                    status: "FAILED",
                    warnings: [
                      {
                        code: "DISPATCH_WARNING",
                        message: "Provider returned a partial verification trace.",
                      },
                    ],
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
                  kind: "session_id",
                  provider: "codex",
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
              providerSessionRefs: [
                {
                  id: "provider-session-verify-1",
                  kind: "session_id",
                  provider: "codex",
                },
              ],
              relatedWorkIds: ["work-verify-1"],
              sessionId: storySessionID,
              status: "FAILED",
              statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
              warnings: [
                {
                  code: "DISPATCH_WARNING",
                  message: "Provider returned a partial verification trace.",
                },
              ],
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
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    expect(await canvas.findByText("Execution mode: live")).toBeTruthy();
    expect(
      await canvas.findByText(
        "Provider session: codex / session_id / provider-session-1",
      ),
    ).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Expand dispatch detail for dispatch-failed",
      }),
    );
    expect(await canvas.findByText("Failure detail")).toBeTruthy();
    expect(await canvas.findByText("VERIFY_ASSERTION_FAILED")).toBeTruthy();
    expect(await canvas.findByText("verification_error")).toBeTruthy();
    expect(await canvas.findByText("Expected release manifest checksum.")).toBeTruthy();
    expect(await canvas.findByText("session_id · provider-session-verify-1")).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Expand dispatch detail for dispatch-missing",
      }),
    );
    expect(await canvas.findByText("This dispatch detail is no longer available.")).toBeTruthy();
    expect(
      await canvas.findByText(
        "Provider session: codex / session_id / provider-session-1",
      ),
    ).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Expand dispatch detail for dispatch-error",
      }),
    );
    expect(await canvas.findByText("dispatch boom")).toBeTruthy();
    expect(
      await canvas.findByText(
        "Provider session: codex / session_id / provider-session-1",
      ),
    ).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(storySessionID),
};
