import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { installDashboardBrowserTestShims } from "../../components/dashboard/test-browser-shims";
import type { DashboardSelection } from "../current-selection";
import type { CurrentActivityImportController } from "./current-activity-import-controller";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

describe("WorkflowActivityBentoCard", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });

  it("wraps the React Flow graph without a floating inspector", async () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const selection: DashboardSelection = { kind: "node", nodeId: selectedNode.node_id };
    const importController = {
      activateImport: vi.fn().mockResolvedValue(undefined),
      activationState: { status: "idle" } as const,
      clearActivationError: vi.fn(),
      clearError: vi.fn(),
      closeImportPreview: vi.fn(),
      dropState: { status: "idle" } as const,
      importPreviewState: { status: "idle" } as const,
      onDragEnter: vi.fn(),
      onDragLeave: vi.fn(),
      onDragOver: vi.fn(),
      onDrop: vi.fn(),
    } satisfies CurrentActivityImportController;
    const queryClient = new QueryClient({
      defaultOptions: {
        mutations: {
          retry: false,
        },
        queries: {
          gcTime: Infinity,
          retry: false,
        },
      },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <WorkflowActivityBentoCard
          importController={importController}
          now={Date.parse("2026-04-08T12:00:04Z")}
          selection={selection}
          snapshot={snapshot}
          onSelectWorkItem={vi.fn()}
          onSelectStateNode={vi.fn()}
          onSelectWorkstation={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: "Factory graph" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Work graph viewport" })).toBeTruthy();
    expect(screen.queryByRole("complementary", { name: "Workstation Info" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Collapse inspector" })).toBeNull();
  });
});
