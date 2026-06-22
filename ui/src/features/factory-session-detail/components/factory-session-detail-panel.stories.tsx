import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const sessionID = "session-beta";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
}

export default {
  title: "Agent Factory/Factory Session Detail/Panel",
  component: FactorySessionDetailPanel,
};

export const ArtifactDrilldown = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${sessionID}`,
          response: {
            body: {
              factoryDir: "/workspace/root/beta",
              folderPath: "/workspace/root",
              id: sessionID,
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
                    orchestratorKind: "JAVASCRIPT",
                    sessionId: sessionID,
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
                orchestratorKind: "JAVASCRIPT",
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
          path: `/factory-sessions/${sessionID}/result`,
          response: {
            body: {
              resultArtifactRef: {
                id: "artifact-download",
                kind: "FINAL_RESULT",
                visibility: "CUSTOMER",
              },
              sessionId: sessionID,
              status: "IDLE",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${sessionID}/partial-result`,
          response: {
            body: {
              partialResultArtifactRef: {
                id: "artifact-1",
                kind: "CHILD_RESULT",
                visibility: "CUSTOMER",
              },
              phase: "review",
              sessionId: sessionID,
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${sessionID}/artifacts/artifact-1`,
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
              sessionId: sessionID,
              sizeBytes: 128,
              summary: "Captured during review",
              visibility: "CUSTOMER",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${sessionID}/artifacts/artifact-download`,
          response: {
            body: {
              auditMode: "OFF",
              contentRef: {
                href: `/factory-sessions/${sessionID}/artifacts/artifact-download`,
                method: "GET",
              },
              createdAt: "2026-06-08T14:03:00Z",
              id: "artifact-download",
              kind: "FINAL_RESULT",
              label: "bundle export",
              sessionId: sessionID,
              visibility: "CUSTOMER",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${sessionID}/artifacts/artifact-unavailable`,
          response: {
            body: {
              auditMode: "OFF",
              createdAt: "2026-06-08T14:04:00Z",
              id: "artifact-unavailable",
              kind: "TRACE_EXPORT",
              label: "trace snapshot",
              sessionId: sessionID,
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
        "Inline preview is unavailable for this durable artifact. Download the artifact to inspect it.",
      ),
    ).toBeTruthy();
    await expect(
      canvas.getByRole("link", { name: "Download artifact" }),
    ).toBeTruthy();

    const unavailableToggle = canvas.getByRole("button", {
      name: "View artifact artifact-unavailable",
    });
    await userEvent.click(unavailableToggle);
    await expect(
      canvas.getByText("Inline preview is unavailable for this durable artifact."),
    ).toBeTruthy();
  },
  render: () => (
    <div style={{ maxWidth: "960px" }}>
      <QueryClientProvider client={createQueryClient()}>
        <FactorySessionDetailPanel sessionID={sessionID} />
      </QueryClientProvider>
    </div>
  ),
};
