import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const storySessionID = "session-beta";

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

export const ArtifactDrilldown = {
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
                artifacts: [
                  {
                    id: "artifact-1",
                    kind: "CHILD_RESULT",
                    label: "review output",
                    visibility: "CUSTOMER",
                  },
                  {
                    id: "artifact-download",
                    kind: "FINAL_RESULT",
                    label: "bundle export",
                    visibility: "CUSTOMER",
                  },
                  {
                    id: "artifact-unavailable",
                    kind: "TRACE_EXPORT",
                    label: "trace snapshot",
                    visibility: "CUSTOMER",
                  },
                ],
                dispatches: [
                  {
                    dispatchKind: "JAVASCRIPT_AGENT",
                    id: "dispatch-1",
                    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                    sessionId: storySessionID,
                    status: "COMPLETED",
                    warnings: [],
                  },
                ],
                javascript: {
                  checkpoints: [],
                  childDispatchCounts: {
                    completed: 2,
                    queued: 0,
                    running: 0,
                  },
                  phase: "review",
                  phases: ["review", "finalize"],
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
            body: {
              resultArtifactRef: {
                id: "artifact-download",
                kind: "FINAL_RESULT",
                visibility: "CUSTOMER",
              },
              sessionId: storySessionID,
              status: "IDLE",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/partial-result`,
          response: {
            body: {
              partialResultArtifactRef: {
                id: "artifact-1",
                kind: "CHILD_RESULT",
                visibility: "CUSTOMER",
              },
              phase: "review",
              sessionId: storySessionID,
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/artifacts/artifact-1`,
          response: {
            body: {
              auditMode: "OFF",
              captureMetadata: {
                capturedAt: "2026-06-08T14:02:30Z",
                mimeType: "text/plain",
                sourceDispatchId: "dispatch-parent",
              },
              content: [{ text: "review output body", type: "output_text" }],
              contentHash: "sha256:artifact-preview-1",
              createdAt: "2026-06-08T14:03:00Z",
              dispatchId: "dispatch-1",
              id: "artifact-1",
              kind: "CHILD_RESULT",
              label: "review output",
              sessionId: storySessionID,
              sizeBytes: 128,
              summary: "Captured during review",
              visibility: "CUSTOMER",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/artifacts/artifact-download`,
          response: {
            body: {
              auditMode: "OFF",
              contentRef: {
                href: `/factory-sessions/${storySessionID}/artifacts/artifact-download`,
                method: "GET",
              },
              createdAt: "2026-06-08T14:03:00Z",
              id: "artifact-download",
              kind: "FINAL_RESULT",
              label: "bundle export",
              sessionId: storySessionID,
              visibility: "CUSTOMER",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${storySessionID}/artifacts/artifact-unavailable`,
          response: {
            body: {
              auditMode: "OFF",
              createdAt: "2026-06-08T14:04:00Z",
              id: "artifact-unavailable",
              kind: "TRACE_EXPORT",
              label: "trace snapshot",
              sessionId: storySessionID,
              visibility: "CUSTOMER",
            },
          },
        },
      ],
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const previewToggle = await canvas.findByRole("button", {
      name: "View artifact artifact-1",
    });
    await userEvent.click(previewToggle);
    await expect(canvas.getByText("Artifact detail")).toBeTruthy();
    await expect(canvas.getByText("Captured during review")).toBeTruthy();

    const downloadToggle = canvas.getByRole("button", {
      name: "View artifact artifact-download",
    });
    await userEvent.click(downloadToggle);
    await expect(
      canvas.getByText(
        "Inline preview is unavailable for this durable artifact, and this session detail route does not expose a downloadable payload yet.",
      ),
    ).toBeTruthy();
    await expect(
      canvas.queryByRole("link", { name: "Download artifact" }),
    ).toBeNull();

    const unavailableToggle = canvas.getByRole("button", {
      name: "View artifact artifact-unavailable",
    });
    await userEvent.click(unavailableToggle);
    await expect(
      canvas.getByText("Inline preview is unavailable for this durable artifact."),
    ).toBeTruthy();
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
      <div style={{ maxWidth: "960px" }}>
        <QueryClientProvider client={queryClient}>
          <FactorySessionDetailPanel sessionID={storySessionID} />
        </QueryClientProvider>
      </div>
    );
  },
};
