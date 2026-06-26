import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const storySessionID = "session-beta";
const durableReplayStorySessionID = "dur-sess-js-success-002";

export default {
  title: "you-agent-factory/Current Selection/Factory Session Detail Panel",
  component: FactorySessionDetailPanel,
};

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
  render: () => {
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
          <FactorySessionDetailPanel sessionID={storySessionID} />
        </QueryClientProvider>
      </div>
    );
  },
};

export const DurableReplayDisclosure = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${durableReplayStorySessionID}`,
          response: {
            body: {
              artifactRefs: [
                {
                  id: "art-js-success-001",
                  kind: "FINAL_RESULT",
                  label: "Docs refresh output",
                  visibility: "PUBLIC",
                },
              ],
              dialect: "you-workflow-v1",
              lifecycle: {
                finishedAt: "2026-06-08T13:10:00Z",
                startedAt: "2026-06-08T13:00:02Z",
              },
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              progress: {
                completedDispatches: 2,
                failedDispatches: 0,
                inFlightDispatches: 0,
                totalDispatches: 2,
              },
              resolvedSource: {
                kind: "WORKFLOW_FILE",
                sourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
                sourceHash: "sha256:js-workflow-docs-refresh",
              },
              resultSummary: {
                resultStatus: "FINAL",
                summary: "Documentation refresh complete.",
              },
              sessionId: durableReplayStorySessionID,
              status: "SUCCEEDED",
              usage: { resources: [] },
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${durableReplayStorySessionID}/events`,
          response: {
            body: [
              'data: {"id":"evt-1","type":"SESSION_STARTED","context":{"sequence":1,"tick":1,"eventTime":"2026-06-25T10:00:00Z","sessionId":"dur-sess-js-success-002","sessionSequence":1,"phaseName":"plan"},"payload":{"startedAt":"2026-06-25T10:00:00Z"}}',
              "",
              'data: {"id":"evt-2","type":"ORCHESTRATOR_PHASE_CHANGED","context":{"sequence":2,"tick":2,"eventTime":"2026-06-25T10:00:01Z","sessionId":"dur-sess-js-success-002","sessionSequence":2,"phaseName":"review"},"payload":{"phase":"review"}}',
              "",
              'data: {"id":"evt-3","type":"DISPATCH_QUEUED","context":{"sequence":3,"tick":3,"eventTime":"2026-06-25T10:00:02Z","sessionId":"dur-sess-js-success-002","sessionSequence":3,"phaseName":"review","dispatchId":"dispatch-1","workIds":["work-1","work-2"]},"payload":{"dispatchKind":"JAVASCRIPT_AGENT"}}',
              "",
            ].join("\n"),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${durableReplayStorySessionID}/dispatches`,
          response: {
            body: {
              dispatches: [
                {
                  dispatchKind: "JAVASCRIPT_AGENT",
                  id: "dispatch-1",
                  outputArtifactIds: [],
                  phase: "review",
                  status: "COMPLETED",
                },
              ],
              sessionId: durableReplayStorySessionID,
            },
          },
        },
      ],
      sessionID: durableReplayStorySessionID,
    },
  },
  render: () => {
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
          <FactorySessionDetailPanel sessionID={durableReplayStorySessionID} />
        </QueryClientProvider>
      </div>
    );
  },
};
