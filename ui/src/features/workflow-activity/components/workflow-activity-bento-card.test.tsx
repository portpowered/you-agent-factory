import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  useCurrentEditableFactoryDefinitionDocument,
  useSaveCurrentEditableFactoryDefinition,
} from "../../current-factory-definition";
import { useFactoryGraphDraftState } from "../../factory-graph-editor/factory-graph-draft";
import type { DashboardSelection } from "../../current-selection";
import type { CurrentActivityImportController } from "../current-activity-import-controller";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";

vi.mock("../../current-factory-definition", async () => {
  const actual = await vi.importActual("../../current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinitionDocument: vi.fn(),
    useSaveCurrentEditableFactoryDefinition: vi.fn(),
  };
});

vi.mock("../../factory-graph-editor/factory-graph-draft", async () => {
  const actual = await vi.importActual(
    "../../factory-graph-editor/factory-graph-draft",
  );

  return {
    ...actual,
    useFactoryGraphDraftState: vi.fn(),
  };
});

const defaultDraftState = {
  baseDocument: null,
  draft: {
    additions: {
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
    edgeChanges: {
      additions: [],
      removals: [],
    },
    removals: {
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
  },
  graph: {
    edges: [],
    nodes: [],
  },
  hasChanges: false,
  latestDocument: null,
  pendingFactoryDefinition: null,
  replaceDraft: vi.fn(),
  resetDraft: vi.fn(),
  source: "projection" as const,
  updateDraft: vi.fn(),
  validationErrors: [],
};

describe("WorkflowActivityBentoCard", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    vi.mocked(useCurrentEditableFactoryDefinitionDocument).mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);
    vi.mocked(useSaveCurrentEditableFactoryDefinition).mockReturnValue({
      mutateAsync: vi.fn(),
      reset: vi.fn(),
      status: "idle",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue(
      defaultDraftState as never,
    );
  });

  afterEach(() => {
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.clearAllMocks();
  });

  it("wraps the React Flow graph without a floating inspector", async () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const selection: DashboardSelection = { kind: "node", nodeId: selectedNode.node_id };
    const locale = "zh-CN";
    const messages = getWorkflowActivityShellMessages(locale);
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
          locale={locale}
          now={Date.parse("2026-04-08T12:00:04Z")}
          selection={selection}
          snapshot={snapshot}
          onSelectWorkItem={vi.fn()}
          onSelectStateNode={vi.fn()}
          onSelectWorkstation={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: "工厂图" })).toBeTruthy();
    expect(
      screen.getByRole("region", { name: messages.viewportLabel }),
    ).toBeTruthy();
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(screen.queryByRole("button", { name: /collapse inspector/i })).toBeNull();
  });
});
