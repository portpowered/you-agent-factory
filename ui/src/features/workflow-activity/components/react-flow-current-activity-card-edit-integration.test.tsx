// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/nursery/noExcessiveLinesPerFile: integration flows share one mocked React Flow harness.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactElement } from "react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} from "../../current-factory-definition/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");

  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      edges,
      isValidConnection,
      nodes,
      onConnect,
      onEdgeClick,
      onNodeClick,
    }: {
      children: React.ReactNode;
      edges?: Array<{
        id: string;
        source?: string;
        sourceHandle?: string | null;
        target?: string;
        targetHandle?: string | null;
      }>;
      isValidConnection?: (connection: {
        source?: string | null;
        sourceHandle?: string | null;
        target?: string | null;
        targetHandle?: string | null;
      }) => boolean;
      nodes?: Array<{
        data?: Record<string, unknown>;
        id: string;
      }>;
      onConnect?: (connection: {
        source?: string | null;
        sourceHandle?: string | null;
        target?: string | null;
        targetHandle?: string | null;
      }) => void;
      onEdgeClick?: (_event: unknown, edge: { id: string }) => void;
      onNodeClick?: (_event: unknown, node: { id: string }) => void;
    }) => {
      const workstationNodeId =
        nodes?.find((node) => node.id.startsWith("workstation:"))?.id ??
        "workstation:review";

      return (
        <div data-testid="mock-react-flow">
          <ul aria-label="Rendered graph nodes">
            {(nodes ?? []).map((node) => (
              <li key={node.id}>
                <button
                  data-factory-graph-node-id={String(
                    node.data?.factoryGraphNodeId ?? "",
                  )}
                  onClick={() => onNodeClick?.(null, { id: node.id })}
                  type="button"
                >
                  {node.id}
                </button>
              </li>
            ))}
          </ul>
          <ul aria-label="Rendered graph edges">
            {(edges ?? []).map((edge) => (
              <li key={edge.id}>
                <button
                  data-source-handle={edge.sourceHandle ?? ""}
                  data-target-handle={edge.targetHandle ?? ""}
                  onClick={() => onEdgeClick?.(null, { id: edge.id })}
                  type="button"
                >
                  {edge.id}
                </button>
              </li>
            ))}
          </ul>
          <output data-testid="valid-qa-output-connection">
            {String(
              isValidConnection?.({
                source: workstationNodeId,
                sourceHandle: "workstation-output-source",
                target: "work-state:story:qa",
                targetHandle: "work-state-input-target",
              }) ?? false,
            )}
          </output>
          <button
            onClick={() =>
              onConnect?.({
                source: workstationNodeId,
                sourceHandle: "workstation-output-source",
                target: "work-state:story:qa",
                targetHandle: "work-state-input-target",
              })
            }
            type="button"
          >
            Mock connect review to QA
          </button>
          <button
            onClick={() =>
              onConnect?.({
                source: workstationNodeId,
                sourceHandle: "workstation-output-source",
                target: "work-state:story:qa",
                targetHandle: "workstation-input-target",
              })
            }
            type="button"
          >
            Mock invalid review connection
          </button>
          {children}
        </div>
      );
    },
  };
});

vi.mock("../../current-factory-definition/public", async () => {
  const actual = await vi.importActual(
    "../../current-factory-definition/public",
  );

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
    useSaveCurrentFactory: vi.fn(),
  };
});

const editableFactoryDocument: CurrentFactoryDocument = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      body: "Review the work item.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "review",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "qa",
          type: "PROCESSING",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  version: {
    logical: "8",
    physical: "2026-05-18T15:32:00Z",
  },
};

function loadSampleFactoryDocument(): CurrentFactoryDocument {
  return {
    ...JSON.parse(
      readFileSync(
        resolve(
          process.cwd(),
          "src/features/workflow-activity/lib/current-activity-sample-factory.fixture.json",
        ),
        "utf-8",
      ),
    ),
    version: {
      logical: "sample",
      physical: "2026-05-28T00:00:00Z",
    },
  } as CurrentFactoryDocument;
}

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

let restoreBrowserTestShims: (() => void) | null = null;
let mutateAsync: ReturnType<typeof vi.fn>;

beforeEach(() => {
  window.localStorage.clear();
  useCurrentActivityGraphStore.setState({ positionsByGraphKey: {} });
  restoreBrowserTestShims = installDashboardBrowserTestShims();
  vi.mocked(useCurrentFactoryDocument).mockReturnValue({
    data: editableFactoryDocument,
    error: null,
    status: "success",
  } as never);
  mutateAsync = vi.fn().mockResolvedValue(editableFactoryDocument);
  vi.mocked(useSaveCurrentFactory).mockReturnValue({
    mutateAsync,
    reset: vi.fn(),
    status: "idle",
  } as never);
});

afterEach(() => {
  cleanup();
  restoreBrowserTestShims?.();
  restoreBrowserTestShims = null;
  vi.clearAllMocks();
});

describe("ReactFlowCurrentActivityCard edit integration", () => {
  it("renders observe-mode graph edges with semantic React Flow handles", async () => {
    renderCurrentActivity();

    const edge = await screen.findByRole("button", {
      name: "workstation-output:workstation:review->work-state:story:done",
    });

    expect(edge.getAttribute("data-source-handle")).toBe(
      "workstation-output-source",
    );
    expect(edge.getAttribute("data-target-handle")).toBe(
      "work-state-input-target",
    );
    expect(edge.getAttribute("data-source-handle")).not.toMatch(/^out-/);
    expect(edge.getAttribute("data-target-handle")).not.toMatch(/^in-/);
  });

  it("renders newly added graph nodes after add-node interactions", async () => {
    renderCurrentActivity();
    enterEditorMode();

    fireEvent.click(
      await screen.findByRole("button", { name: "Open add entity menu" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Work type" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "essay" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "First state" }), {
      target: { value: "queued" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(
      await screen.findByRole("button", { name: "work-state:essay:queued" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "work-type:essay" }),
    ).toBeTruthy();
  });

  it.each([
    {
      expectedNodeNames: ["resource:sandbox-slot"],
      fields: [{ label: "Identifier", value: "sandbox-slot" }],
      menuAction: "Resource",
    },
    {
      expectedNodeNames: ["worker:analyst"],
      fields: [
        { label: "Identifier", value: "analyst" },
        { label: "Model", value: "gpt-5.1" },
      ],
      menuAction: "Worker",
    },
    {
      expectedNodeNames: ["work-type:incident", "work-state:incident:open"],
      fields: [
        { label: "Identifier", value: "incident" },
        { label: "First state", value: "open" },
      ],
      menuAction: "Work type",
    },
    {
      expectedNodeNames: ["work-state:task:blocked"],
      fields: [{ label: "Identifier", value: "blocked" }],
      menuAction: "Work state",
    },
    {
      expectedNodeNames: ["workstation:summarize"],
      fields: [
        { label: "Identifier", value: "summarize" },
        { label: "Prompt body", value: "Summarize the task state." },
      ],
      menuAction: "Workstation",
    },
  ])("renders newly added $menuAction nodes from the sample factory toolbar flow", async ({
    expectedNodeNames,
    fields,
    menuAction,
  }) => {
    const sampleFactoryDocument = loadSampleFactoryDocument();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: sampleFactoryDocument,
      error: null,
      status: "success",
    } as never);

    renderCurrentActivity(createSnapshot(sampleFactoryDocument));
    enterEditorMode();

    fireEvent.click(
      await screen.findByRole("button", { name: "Open add entity menu" }),
    );
    fireEvent.click(screen.getByRole("button", { name: menuAction }));
    for (const field of fields) {
      fireEvent.change(screen.getByRole("textbox", { name: field.label }), {
        target: { value: field.value },
      });
    }
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    for (const expectedNodeName of expectedNodeNames) {
      expect(
        await screen.findByRole("button", { name: expectedNodeName }),
      ).toBeTruthy();
    }
  });

  it("removes a graph node from the rendered graph without delete confirmation", async () => {
    renderCurrentActivity();
    enterEditorMode();
    expect(
      await screen.findByRole("button", { name: "workstation:review" }),
    ).toBeTruthy();

    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    fireEvent.click(screen.getByRole("button", { name: "workstation:review" }));
    expect(
      screen.queryByRole("dialog", {
        name: "Remove review workstation?",
      }),
    ).toBeNull();

    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "workstation:review" }),
      ).toBeNull();
    });
  });

  it("creates a visible semantic React Flow edge after a connect interaction", async () => {
    renderCurrentActivity();
    enterEditorMode();

    await screen.findByRole("button", { name: "work-state:story:qa" });
    fireEvent.click(await screen.findByRole("button", { name: "Connect" }));

    expect(screen.getByTestId("valid-qa-output-connection").textContent).toBe(
      "true",
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Mock connect review to QA" }),
    );

    const edge = await screen.findByRole("button", {
      name: "workstation-output:workstation:review->work-state:story:qa",
    });
    expect(edge.getAttribute("data-source-handle")).toBe(
      "workstation-output-source",
    );
    expect(edge.getAttribute("data-target-handle")).toBe(
      "work-state-input-target",
    );
  });

  it("shows validation feedback and leaves graph edges unchanged for invalid connects", async () => {
    renderCurrentActivity();
    enterEditorMode();
    fireEvent.click(await screen.findByRole("button", { name: "Connect" }));

    expect(
      screen.queryByRole("button", {
        name: "workstation-output:workstation:review->work-state:story:qa",
      }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Mock invalid review connection" }),
    );

    expect(await screen.findByText("Connection blocked")).toBeTruthy();
    expect(
      screen.getByText(
        "Choose a compatible source and target anchor before creating a connection.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "workstation-output:workstation:review->work-state:story:qa",
      }),
    ).toBeNull();
  });
});

describe("ReactFlowCurrentActivityCard distinct workstation ID editing", () => {
  it("keeps workstation editor connections stable when id differs from name", async () => {
    const factoryDocumentWithDistinctWorkstationId = {
      ...editableFactoryDocument,
      workstations: [
        {
          ...editableFactoryDocument.workstations[0],
          id: "canonical-review-id",
          name: "review",
        },
      ],
    } satisfies CurrentFactoryDocument;
    mutateAsync.mockResolvedValue(factoryDocumentWithDistinctWorkstationId);
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: factoryDocumentWithDistinctWorkstationId,
      error: null,
      status: "success",
    } as never);
    renderCurrentActivity(
      createSnapshot(factoryDocumentWithDistinctWorkstationId),
    );
    enterEditorMode();

    expect(
      await screen.findByRole("button", { name: "workstation:review" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "workstation:canonical-review-id" }),
    ).toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: "Connect" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Mock connect review to QA" }),
    );

    expect(
      await screen.findByRole("button", {
        name: "workstation-output:workstation:review->work-state:story:qa",
      }),
    ).toBeTruthy();

    fireEvent.click(
      within(
        await screen.findByRole("region", {
          name: "Factory graph editor tools",
        }),
      ).getByRole("button", { name: "Save changes" }),
    );
    fireEvent.click(
      within(
        await screen.findByRole("dialog", {
          name: "Save factory graph changes?",
        }),
      ).getByRole("button", { name: "Save topology" }),
    );

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        baseVersion: factoryDocumentWithDistinctWorkstationId.version,
        factoryDefinition: {
          ...factoryDocumentWithDistinctWorkstationId,
          resources: [],
          workstations: [
            {
              ...factoryDocumentWithDistinctWorkstationId.workstations[0],
              outputs: [
                ...factoryDocumentWithDistinctWorkstationId.workstations[0]
                  .outputs,
                {
                  state: "qa",
                  workType: "story",
                },
              ],
            },
          ],
        },
      });
    });
  });
});

function renderCurrentActivity(snapshot = createSnapshot()) {
  renderWithQueryClient(
    <ReactFlowCurrentActivityCard
      importController={importController}
      now={Date.parse("2026-04-08T12:00:04Z")}
      onSelectStateNode={vi.fn()}
      onSelectWorkID={vi.fn()}
      onSelectWorker={vi.fn()}
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
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}

function createSnapshot(
  factoryDocument: CurrentFactoryDocument = editableFactoryDocument,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.factory = factoryDocument;
  snapshot.runtime.in_flight_dispatch_count = 0;
  return snapshot;
}

function enterEditorMode() {
  fireEvent.click(
    screen.getByRole("button", { name: "Enter factory graph editor" }),
  );
}
