import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, within } from "storybook/test";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const durableJavaScriptSessionID = "dur-sess-js-run-n-001";
const durableJavaScriptMissingSessionID = "dur-sess-js-missing-001";
const durableJavaScriptErrorSessionID = "dur-sess-js-error-001";

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
    <QueryClientProvider client={queryClient}>
      <FactorySessionDetailPanel sessionID={sessionID} />
    </QueryClientProvider>
  );
}

export default {
  title: "you-agent-factory/Current Selection/Factory Session Detail Panel",
  component: FactorySessionDetailPanel,
};

export const DurableJavaScriptSessionInspectionDetails = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions/dur-sess-js-success-002",
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
                dialect: "you-workflow-v1",
                kind: "WORKFLOW_FILE",
                sourceHash: "sha256:js-workflow-docs-refresh",
                sourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
              },
              resultSummary: {
                resultStatus: "FINAL",
                summary: "Documentation refresh complete.",
              },
              sessionId: "dur-sess-js-success-002",
              sourceHash: "sha256:js-workflow-docs-refresh",
              status: "SUCCEEDED",
              usage: { resources: [] },
            },
          },
        },
        {
          method: "GET",
          path: "/factory-sessions/dur-sess-js-success-002/dispatches",
          response: {
            body: {
              dispatches: [
                {
                  attempt: 1,
                  dispatchKind: "JAVASCRIPT_AGENT",
                  id: "disp-js-success-001",
                  label: "draft-docs",
                  status: "COMPLETED",
                },
                {
                  attempt: 1,
                  dispatchKind: "JAVASCRIPT_VERIFY",
                  id: "disp-js-success-002",
                  label: "verify-docs",
                  outputArtifactIds: ["art-js-success-001"],
                  status: "COMPLETED",
                  warnings: [
                    {
                      code: "DISPATCH_WARNING",
                      message: "child output truncated for display",
                    },
                  ],
                },
              ],
              sessionId: "dur-sess-js-success-002",
            },
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => renderFactorySessionDetailPanel("dur-sess-js-success-002"),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("heading", { name: "Factory session runtime" }),
    ).toBeVisible();
    await expect(canvas.findByText("Child dispatch activity")).resolves.toBeVisible();
    await expect(
      canvas.findByText("disp-js-success-002 (verify-docs) · COMPLETED"),
    ).resolves.toBeVisible();
    await expect(
      canvas.findByText("child output truncated for display"),
    ).resolves.toBeVisible();
    await expect(canvas.findByText("art-js-success-001 · FINAL_RESULT")).resolves.toBeVisible();
  },
};

export const DurableJavaScriptSessionSummary = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${durableJavaScriptSessionID}`,
          response: {
            body: {
              dialect: "you-workflow-v1",
              lifecycle: {
                startedAt: "2026-06-08T14:00:00Z",
                updatedAt: "2026-06-08T14:05:00Z",
              },
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              phase: "verify",
              phaseSummaries: [
                { completedDispatchCount: 1, dispatchCount: 1, phase: "plan" },
                { dispatchCount: 2, phase: "verify" },
              ],
              progress: {
                completedDispatches: 1,
                failedDispatches: 0,
                inFlightDispatches: 1,
                totalDispatches: 3,
              },
              resolvedSource: {
                dialect: "you-workflow-v1",
                kind: "WORKFLOW_NAME",
                sourceHash: "sha256:js-workflow-release-train",
                sourceRef: "workflow/release-train",
              },
              sessionId: durableJavaScriptSessionID,
              sourceHash: "sha256:js-workflow-release-train",
              status: "RUNNING",
              usage: { resources: [] },
            },
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => renderFactorySessionDetailPanel(durableJavaScriptSessionID),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("heading", { name: "Factory session runtime" }),
    ).toBeVisible();
    await expect(canvas.findByText("JavaScript workflow")).resolves.toBeVisible();
    await expect(canvas.findAllByText("Running")).resolves.toHaveLength(2);
    await expect(canvas.findByText("verify")).resolves.toBeVisible();
    await expect(canvas.findByText("plan, verify")).resolves.toBeVisible();
  },
};

export const DurableJavaScriptSessionLoading = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${durableJavaScriptSessionID}`,
          response: () => new Promise<never>(() => undefined),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => renderFactorySessionDetailPanel(durableJavaScriptSessionID),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("heading", { name: "Factory session runtime" }),
    ).toBeVisible();
    await expect(canvas.getByRole("status")).toHaveTextContent(
      "Loading factory session runtime…",
    );
    await expect(canvas.getByText(durableJavaScriptSessionID)).toBeVisible();
  },
};

export const DurableJavaScriptSessionNotFound = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${durableJavaScriptMissingSessionID}`,
          response: {
            body: {
              code: "NOT_FOUND",
              message: "Factory session not found.",
            },
            status: 404,
            statusText: "Not Found",
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => renderFactorySessionDetailPanel(durableJavaScriptMissingSessionID),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("heading", { name: "Factory session runtime" }),
    ).toBeVisible();
    await expect(canvas.findByRole("status")).resolves.toHaveTextContent(
      "This factory session is no longer available.",
    );
    await expect(canvas.queryByText("Runtime")).toBeNull();
  },
};

export const DurableJavaScriptSessionError = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${durableJavaScriptErrorSessionID}`,
          response: {
            body: {
              code: "INTERNAL_ERROR",
              message: "Factory session runtime unavailable.",
            },
            status: 500,
            statusText: "Internal Server Error",
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => renderFactorySessionDetailPanel(durableJavaScriptErrorSessionID),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("heading", { name: "Factory session runtime" }),
    ).toBeVisible();
    await expect(canvas.findByRole("alert")).resolves.toHaveTextContent(
      "Factory session runtime unavailable.",
    );
  },
};
