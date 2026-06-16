import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, within } from "storybook/test";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const durableJavaScriptSessionID = "dur-sess-js-run-n-001";

export default {
  title: "you-agent-factory/Current Selection/Factory Session Detail Panel",
  component: FactorySessionDetailPanel,
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
      <QueryClientProvider client={queryClient}>
        <FactorySessionDetailPanel sessionID={durableJavaScriptSessionID} />
      </QueryClientProvider>
    );
  },
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
