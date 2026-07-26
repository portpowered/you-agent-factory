// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/style/noExcessiveLinesPerFile: integration flows share one mocked React Flow harness.
import "../../../testing/vitest-dom-capabilities.setup";

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
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { DashboardSessionTestProvider } from "../../../testing/dashboard-session-test-provider";
import { selectLabeledComboboxOption } from "../../../testing/select-test-helpers";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../current-factory-definition/hooks/useFactoryDocumentSave";
import { materializeFactoryGraphEntityIdsForSave } from "../../factory-graph-editor/lib/operations/factory-graph-public-ids";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useGraphEditorPendingFactoryBridge } from "../state/graph-editor-pending-factory-bridge";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";

vi.mock("@you-agent-factory/components/overlays", async (importOriginal) => {
  const actual =
    await importOriginal<
      typeof import("@you-agent-factory/components/overlays")
    >();
  const mockDialog = await import("../../../testing/mock-dashboard-dialog");
  return {
    ...actual,
    Dialog: mockDialog.Dialog,
    DialogContent: mockDialog.DialogContent,
    DialogDescription: mockDialog.DialogDescription,
    DialogFooter: mockDialog.DialogFooter,
    DialogHeader: mockDialog.DialogHeader,
    DialogOverlay: mockDialog.DialogOverlay,
    DialogPortal: mockDialog.DialogPortal,
    DialogTitle: mockDialog.DialogTitle,
  };
});

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");
  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      defaultViewport,
      edges,
      isValidConnection,
      nodes,
      onConnect,
      onEdgeClick,
      onNodeClick,
      onSelectionChange,
    }: {
      children: React.ReactNode;
      defaultViewport?: { x: number; y: number; zoom: number };
      edges?: Array<{
        data?: Record<string, unknown>;
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
        position?: { x: number; y: number };
      }>;
      onConnect?: (connection: {
        source?: string | null;
        sourceHandle?: string | null;
        target?: string | null;
        targetHandle?: string | null;
      }) => void;
      onEdgeClick?: (_event: unknown, edge: { id: string }) => void;
      onNodeClick?: (_event: unknown, node: { id: string }) => void;
      onSelectionChange?: (selection: {
        edges: Array<{ id: string }>;
        nodes: Array<{ id: string }>;
      }) => void;
    }) => {
      const workstationNodeId =
        nodes?.find((node) => node.id.startsWith("workstation:"))?.id ??
        "workstation:review";

      return (
        <div
          data-edges={JSON.stringify(edges ?? [])}
          data-nodes={JSON.stringify(nodes ?? [])}
          data-testid="mock-react-flow"
          data-viewport={JSON.stringify(defaultViewport ?? null)}
        >
          <ul aria-label="Rendered graph nodes">
            {(nodes ?? []).map((node) => (
              <li key={node.id}>
                <button
                  data-factory-graph-node-id={String(
                    node.data?.factoryGraphNodeId ?? "",
                  )}
                  onClick={() => {
                    onSelectionChange?.({
                      edges: [],
                      nodes: [{ id: node.id }],
                    });
                    onNodeClick?.(null, { id: node.id });
                  }}
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
let saveAsync: ReturnType<typeof vi.fn>;

beforeEach(() => {
  window.localStorage.clear();
  restoreBrowserTestShims = installDashboardBrowserTestShims();
  vi.mocked(useCurrentFactoryDocument).mockReturnValue({
    data: editableFactoryDocument,
    error: null,
    status: "success",
  } as never);
  saveAsync = vi.fn().mockResolvedValue(editableFactoryDocument);
  vi.mocked(useFactoryDocumentSave).mockReturnValue({
    error: null,
    isPending: false,
    reset: vi.fn(),
    save: vi.fn(),
    saveAsync,
  } as never);
});

afterEach(() => {
  cleanup();
  restoreBrowserTestShims?.();
  restoreBrowserTestShims = null;
  vi.clearAllMocks();
});

describe("ReactFlowCurrentActivityCard edit integration", () => {
  it("renders newly added graph nodes after add-node interactions", async () => {
    renderCurrentActivity();
    enterEditorMode();

    fireEvent.click(await screen.findByRole("button", { name: "Add" }));
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
        {
          label: "Worker type",
          optionLabel: "Script worker",
          role: "combobox" as const,
          value: "SCRIPT_WORKER",
        },
        { label: "Command", value: "./analyze.sh" },
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
  ])(
    "renders newly added $menuAction nodes from the sample factory toolbar flow",
    async ({ expectedNodeNames, fields, menuAction }) => {
      const sampleFactoryDocument = loadSampleFactoryDocument();
      vi.mocked(useCurrentFactoryDocument).mockReturnValue({
        data: sampleFactoryDocument,
        error: null,
        status: "success",
      } as never);

      renderCurrentActivity(createSnapshot(sampleFactoryDocument));
      enterEditorMode();

      fireEvent.click(await screen.findByRole("button", { name: "Add" }));
      fireEvent.click(screen.getByRole("button", { name: menuAction }));
      const user = userEvent.setup();
      for (const field of fields) {
        if ("role" in field && field.role === "combobox") {
          await selectLabeledComboboxOption(
            user,
            field.label,
            "optionLabel" in field && field.optionLabel
              ? field.optionLabel
              : field.value,
          );
          continue;
        }

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
    },
  );

  it("confirms workstation deletion before removing it from the rendered graph", async () => {
    renderCurrentActivity();
    enterEditorMode();
    expect(
      await screen.findByRole("button", { name: "workstation:review" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "workstation:review" }));
    fireEvent.click(
      await screen.findByRole("button", { name: "Delete selected graph item" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Remove review workstation?",
    });
    expect(
      within(dialog).getByText(/This will remove \d+ graph edges/i),
    ).toBeTruthy();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Delete review workstation" }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "workstation:review" }),
      ).toBeNull();
    });

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(toolbar)
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("enables save after confirming work-state deletion", async () => {
    renderCurrentActivity();
    enterEditorMode();
    expect(
      await screen.findByRole("button", { name: "work-state:story:qa" }),
    ).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "work-state:story:qa" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Delete selected graph item" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Remove story:qa work-state?",
    });
    fireEvent.click(
      within(dialog).getByRole("button", {
        name: "Delete story:qa work-state",
      }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "work-state:story:qa" }),
      ).toBeNull();
    });

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(toolbar)
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("enables save after confirming connected work-state deletion", async () => {
    renderCurrentActivity();
    enterEditorMode();
    expect(
      await screen.findByRole("button", { name: "work-state:story:queued" }),
    ).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "work-state:story:queued" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Delete selected graph item" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Remove story:queued work-state?",
    });
    fireEvent.click(
      within(dialog).getByRole("button", {
        name: "Delete story:queued work-state",
      }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "work-state:story:queued" }),
      ).toBeNull();
    });

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(toolbar)
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("enables save after confirming work-type deletion", async () => {
    renderCurrentActivity();
    enterEditorMode();
    expect(
      await screen.findByRole("button", { name: "work-type:story" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "work-type:story" }));
    fireEvent.click(
      await screen.findByRole("button", { name: "Delete selected graph item" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Remove story work-type?",
    });
    fireEvent.click(
      within(dialog).getByRole("button", {
        name: "Delete story work-type",
      }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "work-type:story" }),
      ).toBeNull();
    });

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    expect(
      within(toolbar)
        .getByRole("button", { name: "Save changes" })
        .getAttribute("disabled"),
    ).toBeNull();
  });

  it("creates a visible semantic React Flow edge after a connect interaction", async () => {
    renderCurrentActivity();
    enterEditorMode();

    await screen.findByRole("button", { name: "work-state:story:qa" });

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

  it("removes a deleted worker from the graph after save adopts the saved factory document", async () => {
    const factoryWithSpareWorker: CurrentFactoryDocument = {
      ...editableFactoryDocument,
      workers: [
        ...(editableFactoryDocument.workers ?? []),
        {
          model: "gpt-5-mini",
          name: "spare",
          type: "MODEL_WORKER",
        },
      ],
    };
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: factoryWithSpareWorker,
      error: null,
      status: "success",
    } as never);
    saveAsync.mockImplementation(async (input) => ({
      ...input.factory,
      version: {
        logical: "9",
        physical: "2026-05-31T02:00:00Z",
      },
    }));

    renderCurrentActivity(createSnapshot(factoryWithSpareWorker));
    enterEditorMode();

    expect(
      await screen.findByRole("button", { name: "worker:spare" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "worker:spare" }));
    fireEvent.click(
      await screen.findByRole("button", { name: "Delete selected graph item" }),
    );

    const toolbar = await screen.findByRole("region", {
      name: "Factory graph editor tools",
    });
    fireEvent.click(
      within(toolbar).getByRole("button", { name: "Save changes" }),
    );
    fireEvent.click(
      within(
        await screen.findByRole("dialog", {
          name: "Save factory graph changes?",
        }),
      ).getByRole("button", { name: "Save topology" }),
    );

    await waitFor(() => {
      expect(saveAsync).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "worker:spare" })).toBeNull();
    });
    await waitFor(() => {
      expect(
        within(toolbar)
          .getByRole("button", { name: "Discard changes" })
          .getAttribute("disabled"),
      ).not.toBeNull();
      expect(
        within(toolbar)
          .getByRole("button", { name: "Save changes" })
          .getAttribute("disabled"),
      ).not.toBeNull();
    });
  });

  it("renders a newly added doc node and exposes it through the pending factory bridge", async () => {
    renderCurrentActivity();
    enterEditorMode();

    fireEvent.click(await screen.findByRole("button", { name: "Add" }));
    fireEvent.click(screen.getByRole("button", { name: "Doc" }));
    fireEvent.change(screen.getByRole("textbox", { name: "File name" }), {
      target: { value: "playbook.md" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Doc text" }), {
      target: { value: "# Playbook\n" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(
      await screen.findByRole("button", {
        name: "doc:factory/docs/playbook.md",
      }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        useGraphEditorPendingFactoryBridge
          .getState()
          .pendingFactoryDefinition?.supportingFiles?.bundledFiles?.some(
            (bundledFile) =>
              bundledFile.type === "DOC" &&
              bundledFile.targetPath === "factory/docs/playbook.md",
          ),
      ).toBe(true);
    });
  });

  it("confirms doc deletion before removing the doc node from the draft graph", async () => {
    const factoryWithDoc: CurrentFactoryDocument = {
      ...editableFactoryDocument,
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide\n" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
        ],
      },
    };
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: factoryWithDoc,
      error: null,
      status: "success",
    } as never);

    renderCurrentActivity(createSnapshot(factoryWithDoc));
    enterEditorMode();

    expect(
      await screen.findByRole("button", { name: "doc:factory/docs/guide.md" }),
    ).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "doc:factory/docs/guide.md" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Delete selected graph item" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Remove guide.md doc?",
    });
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Delete guide.md doc" }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "doc:factory/docs/guide.md" }),
      ).toBeNull();
    });
  });

  it("opens save confirmation from the activity card host portaled to document.body", async () => {
    renderCurrentActivity();
    enterEditorMode();

    await screen.findByRole("button", { name: "work-state:story:qa" });
    fireEvent.click(
      screen.getByRole("button", { name: "Mock connect review to QA" }),
    );

    fireEvent.click(
      within(
        await screen.findByRole("region", {
          name: "Factory graph editor tools",
        }),
      ).getByRole("button", { name: "Save changes" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Save factory graph changes?",
    });
    expect(document.body.contains(dialog)).toBe(true);

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Save topology" }),
    );

    await waitFor(() => {
      expect(saveAsync).toHaveBeenCalledWith({
        baseVersion: editableFactoryDocument.version,
        factory: materializeFactoryGraphEntityIdsForSave({
          ...editableFactoryDocument,
          resources: [],
          workstations: [
            {
              ...editableFactoryDocument.workstations[0],
              outputs: [
                ...editableFactoryDocument.workstations[0].outputs,
                {
                  state: "qa",
                  workType: "story",
                },
              ],
            },
          ],
        }),
      });
    });
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
    saveAsync.mockResolvedValue(factoryDocumentWithDistinctWorkstationId);
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
      await screen.findByRole("button", {
        name: "workstation:canonical-review-id",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "workstation:review" }),
    ).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "Mock connect review to QA" }),
    );

    expect(
      await screen.findByRole("button", {
        name: "workstation-output:workstation:canonical-review-id->work-state:story:qa",
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
      expect(saveAsync).toHaveBeenCalledWith({
        baseVersion: factoryDocumentWithDistinctWorkstationId.version,
        factory: materializeFactoryGraphEntityIdsForSave({
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
        }),
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
      onSelectDoc={vi.fn()}
      onSelectResource={vi.fn()}
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

function createSnapshot(
  factoryDocument: CurrentFactoryDocument = editableFactoryDocument,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.factory = factoryDocument;
  snapshot.runtime.in_flight_dispatch_count = 0;
  return snapshot;
}

function enterEditorMode() {
  fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));
}
