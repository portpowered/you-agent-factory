import "../../../testing/vitest-dom-capabilities.setup";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { Edge, Node } from "@xyflow/react";
import type { ReactElement, ReactNode } from "react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { DashboardSessionTestProvider } from "../../../testing/dashboard-session-test-provider";
import {
  createHookTestGraphEditorDraftState,
  wireMockEditableFactoryGraph,
} from "../../../testing/graph-editor-harness";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import { useEditableFactoryGraph } from "../../factory-graph-editor/hooks/use-editable-factory-graph";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { dashboardWorkstationFromFactory } from "../lib/current-activity-factory-graph-layout";
import { loadFactoryGraphOnrejectionEdgeReproductionFactory } from "../lib/test-data/factory-graph-onrejection-edge-reproduction.fixture";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";

const reportedReactFlowErrors: Array<{ errorId: string; message: string }> = [];

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");

  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      edges,
      nodes,
      onError,
    }: {
      children: ReactNode;
      edges?: Edge[];
      nodes?: Node[];
      onError?: (errorId: string, message: string) => void;
    }) => {
      reportMissingEdgeHandles({
        edges: edges ?? [],
        nodes: nodes ?? [],
        onError,
      });

      return (
        <div data-testid="mock-react-flow">
          <ul aria-label="Rendered graph edges">
            {(edges ?? []).map((edge) => (
              <li key={edge.id}>{edge.id}</li>
            ))}
          </ul>
          {children}
        </div>
      );
    },
  };
});

function reportMissingEdgeHandles({
  edges,
  nodes,
  onError,
}: {
  edges: Edge[];
  nodes: Node[];
  onError?: (errorId: string, message: string) => void;
}) {
  for (const edge of edges) {
    const sourceNode = nodes.find((node) => node.id === edge.source);
    const targetNode = nodes.find((node) => node.id === edge.target);
    if (!sourceNode || !targetNode) {
      const payload = {
        errorId: "006",
        message: `Couldn't create edge for missing node, edge id: ${edge.id}.`,
      };
      reportedReactFlowErrors.push(payload);
      onError?.(payload.errorId, payload.message);
      continue;
    }

    if (!nodeHasHandle(sourceNode, edge.sourceHandle)) {
      const payload = {
        errorId: "008",
        message: `Couldn't create edge for source handle id: "${edge.sourceHandle}", edge id: ${edge.id}.`,
      };
      reportedReactFlowErrors.push(payload);
      onError?.(payload.errorId, payload.message);
      continue;
    }

    if (!nodeHasHandle(targetNode, edge.targetHandle)) {
      const payload = {
        errorId: "008",
        message: `Couldn't create edge for target handle id: "${edge.targetHandle}", edge id: ${edge.id}.`,
      };
      reportedReactFlowErrors.push(payload);
      onError?.(payload.errorId, payload.message);
    }
  }
}

function nodeHasHandle(node: Node, handleId: string | null | undefined) {
  const handles = (node.data as { handles?: Array<{ id: string }> }).handles;
  return Boolean(handleId && handles?.some((handle) => handle.id === handleId));
}

vi.mock(
  "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

vi.mock(
  "../../factory-graph-editor/hooks/use-editable-factory-graph",
  async () => {
    const actual = await vi.importActual(
      "../../factory-graph-editor/hooks/use-editable-factory-graph",
    );

    return {
      ...actual,
      useEditableFactoryGraph: vi.fn(),
    };
  },
);

vi.mock(
  "../../factory-graph-editor/hooks/factory-graph-draft-hook",
  async () => {
    const actual = await vi.importActual(
      "../../factory-graph-editor/hooks/factory-graph-draft-hook",
    );

    return {
      ...actual,
      useFactoryGraphDraftState: vi.fn(),
    };
  },
);

const importController: CurrentActivityImportController = {
  activateImport: vi.fn().mockResolvedValue(undefined),
  activationState: { status: "idle" },
  clearActivationError: vi.fn(),
  clearError: vi.fn(),
  closeImportPreview: vi.fn(),
  dropState: { status: "idle" },
  importPreviewState: { status: "idle" },
  onDragEnter: vi.fn(),
  onDragLeave: vi.fn(),
  onDragOver: vi.fn(),
  onDrop: vi.fn(),
};

function reproductionFactoryDocument(): CurrentFactoryDocument {
  return {
    ...loadFactoryGraphOnrejectionEdgeReproductionFactory(),
    version: {
      logical: "repro-onrejection-edge",
      physical: "2026-05-31T00:00:00Z",
    },
  };
}

function buildReproductionSnapshot(
  factoryDocument: CurrentFactoryDocument,
): DashboardSnapshot {
  const workstations = (factoryDocument.workstations ?? []).map(
    dashboardWorkstationFromFactory,
  );

  return {
    factory: factoryDocument,
    factory_state: "IDLE",
    runtime: {
      active_executions_by_dispatch_id: {},
      current_work_items_by_place_id: {},
      place_occupancy_work_items_by_place_id: {},
      place_token_counts: {},
      session: {
        completed_count: 0,
        dispatched_count: 0,
        failed_count: 0,
        has_data: true,
        provider_sessions: [],
      },
      workstation_requests_by_dispatch_id: {},
    },
    tick_count: 0,
    topology: {
      edges: [],
      workstation_node_ids: workstations.map(
        (workstation) => workstation.node_id,
      ),
      workstation_nodes_by_id: Object.fromEntries(
        workstations.map((workstation) => [workstation.node_id, workstation]),
      ),
    },
    uptime_seconds: 0,
  };
}

let restoreBrowserTestShims: (() => void) | null = null;

beforeEach(() => {
  reportedReactFlowErrors.length = 0;
  window.localStorage.clear();
  restoreBrowserTestShims = installDashboardBrowserTestShims();

  const factoryDocument = reproductionFactoryDocument();
  vi.mocked(useCurrentFactoryDocument).mockReturnValue({
    data: factoryDocument,
    error: null,
    status: "success",
  } as never);
  vi.mocked(useFactoryDocumentSave).mockReturnValue({
    error: null,
    isPending: false,
    reset: vi.fn(),
    save: vi.fn(),
    saveAsync: vi.fn().mockResolvedValue(factoryDocument),
  } as never);
  wireMockEditableFactoryGraph(
    {
      useEditableFactoryGraph: vi.mocked(useEditableFactoryGraph),
      useFactoryGraphDraftState: vi.mocked(useFactoryGraphDraftState),
    },
    createHookTestGraphEditorDraftState({
      baseDocument: factoryDocument,
      latestDocument: factoryDocument,
      pendingFactoryDefinition: factoryDocument,
    }),
  );
});

afterEach(() => {
  cleanup();
  restoreBrowserTestShims?.();
  restoreBrowserTestShims = null;
  vi.clearAllMocks();
});

describe("ReactFlowCurrentActivityCard edit-mode onRejection edge regression", () => {
  it("enters and leaves factory graph editor without React Flow endpoint error 008", async () => {
    const factoryDocument = reproductionFactoryDocument();
    renderCurrentActivity(buildReproductionSnapshot(factoryDocument));

    fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Leave editor" })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "Leave editor" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Edit mode" })).toBeTruthy();
    });

    expect(
      reportedReactFlowErrors.filter((error) => error.errorId === "008"),
    ).toEqual([]);
    expect(
      screen.queryByText(/React Flow factory graph endpoint error 008/),
    ).toBeNull();
  });
});

function renderCurrentActivity(snapshot: DashboardSnapshot) {
  renderWithQueryClient(
    <ReactFlowCurrentActivityCard
      importController={importController}
      now={Date.parse("2026-05-31T00:00:00Z")}
      onSelectStateNode={vi.fn()}
      onSelectWorkID={vi.fn()}
      onSelectWorker={vi.fn()}
      onSelectWorkType={vi.fn()}
      onSelectWorkstation={vi.fn()}
      selection={null}
      snapshot={snapshot}
    />,
  );
}

function renderWithQueryClient(view: ReactElement) {
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

  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTestProvider>{view}</DashboardSessionTestProvider>
    </QueryClientProvider>,
  );
}
