import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, within } from "storybook/test";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const liveSessionID = "session-beta";
const durableSessionID = "dur-sess-petri-success-001";

function renderPanel(sessionID: string, width = "960px") {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return (
    <div style={{ maxWidth: "100%", width }}>
      <QueryClientProvider client={queryClient}>
        <FactorySessionDetailPanel sessionID={sessionID} />
      </QueryClientProvider>
    </div>
  );
}

export default {
  title: "you-agent-factory/Current Selection/Factory Session Detail Panel",
  component: FactorySessionDetailPanel,
};

export const LiveJavaScriptSessionDetail = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${liveSessionID}`,
          response: {
            body: {
              id: liveSessionID,
              runtime: {
                artifacts: [],
                dispatches: [],
                javascript: {
                  checkpoints: [
                    {
                      id: "cp-1",
                      label: "plan",
                      summary: "saved plan checkpoint",
                    },
                  ],
                  childDispatchCounts: {
                    completed: 4,
                    queued: 1,
                    running: 2,
                  },
                  phase: "review",
                  phases: ["plan", "review"],
                  scriptStatus: "IDLE",
                },
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                status: "IDLE",
              },
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${liveSessionID}/result`,
          response: {
            body: {
              resultArtifactRef: {
                id: "artifact-final",
                kind: "FINAL_RESULT",
                visibility: "CUSTOMER",
              },
              sessionId: liveSessionID,
              status: "IDLE",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${liveSessionID}/partial-result`,
          response: {
            body: {
              partialResultArtifactRef: {
                id: "artifact-partial",
                kind: "CHILD_RESULT",
                visibility: "CUSTOMER",
              },
              phase: "review",
              sessionId: liveSessionID,
            },
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Dynamic workflow (JavaScript factory session)");
    expect(canvas.getByText("review")).toBeTruthy();
    expect(canvas.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
    expect(canvas.getByText("artifact-partial · CHILD_RESULT")).toBeTruthy();
    expect(canvas.queryByText("Factory Session complete")).toBeNull();
  },
  render: () => renderPanel(liveSessionID),
};

export const DurableTerminalSessionDetail = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${durableSessionID}`,
          response: {
            body: {
              orchestratorKind: "PETRI",
              progress: {
                completedDispatches: 1,
                totalDispatches: 1,
              },
              resolvedSource: {
                kind: "FACTORY_ID",
                sourceRef: "factory/customer-support-triage",
              },
              resultSummary: {
                resultStatus: "FINAL",
                summary: "Ticket triaged and resolved.",
              },
              sessionId: durableSessionID,
              status: "SUCCEEDED",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${durableSessionID}/results?mode=final`,
          response: {
            body: {
              mode: "final",
              resultStatus: "FINAL",
              sessionId: durableSessionID,
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${durableSessionID}/results?mode=partial`,
          response: {
            body: {
              mode: "partial",
              resultStatus: "PARTIAL",
              sessionId: durableSessionID,
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${durableSessionID}/dispatches`,
          response: {
            body: {
              dispatches: [
                {
                  dispatchKind: "PETRI_TRANSITION",
                  id: "disp-petri-success-001",
                  label: "plan-task",
                  status: "COMPLETED",
                },
              ],
              sessionId: durableSessionID,
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${durableSessionID}/artifacts`,
          response: {
            body: {
              artifacts: [
                {
                  dispatchId: "disp-petri-success-001",
                  id: "art-petri-final-001",
                  kind: "FINAL_RESULT",
                  label: "Triage summary",
                  retrievalRef: {
                    href: `/factory-sessions/${durableSessionID}/artifacts/art-petri-final-001`,
                    method: "GET",
                  },
                  visibility: "PUBLIC",
                },
              ],
              sessionId: durableSessionID,
            },
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Factory Session complete");
    expect(canvas.getByText("Ticket triaged and resolved.")).toBeTruthy();
    expect(canvas.getByText("Dispatches")).toBeTruthy();
    expect(canvas.getByText("Artifacts")).toBeTruthy();
    expect(canvas.getByRole("link", { name: "Inspect Artifact" })).toBeTruthy();
    expect(
      canvas.queryByText("Dynamic workflow (JavaScript factory session)"),
    ).toBeNull();
  },
  render: () => renderPanel(durableSessionID),
};

export const DurableDetailResponsiveLayout = {
  tags: ["test"],
  parameters: DurableTerminalSessionDetail.parameters,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const panel = await canvas.findByLabelText("Factory Session detail");
    expect(panel.getBoundingClientRect().width).toBeGreaterThan(0);
    expect(canvas.getByText("Factory Session complete")).toBeTruthy();
    expect(canvas.getByText("Dispatches")).toBeTruthy();
    expect(canvas.getByText("Artifacts")).toBeTruthy();
  },
  render: () => renderPanel(durableSessionID, "360px"),
};
