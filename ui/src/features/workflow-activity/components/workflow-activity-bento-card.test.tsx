/* biome-ignore lint/style/noExcessiveLinesPerFile: keeps the workflow-activity bento coverage in one file while the toolbar migration settles. */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { DashboardSessionTestProvider } from "../../../testing/dashboard-session-test-provider";
import { baseFactoryDefinitionDocument } from "../../../testing/graph-editor-harness";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../current-factory-definition/hooks/useFactoryDocumentSave";
import type { DashboardSelection } from "../../current-selection/public";
import { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

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

const defaultDraftState = {
  baseDocument: null,
  draft: {
    additions: {
      docs: [],
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
      docs: [],
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

function createImportController(): CurrentActivityImportController {
  return {
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
  };
}

function createQueryClient(): QueryClient {
  return new QueryClient({
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
}

function renderWorkflowActivityBentoCard({
  headerAction,
  locale = "zh-CN",
  widgetInstanceID,
}: {
  headerAction?: ReactNode;
  locale?: string;
  widgetInstanceID?: string;
} = {}) {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
  const selection: DashboardSelection = {
    kind: "node",
    nodeId: selectedNode.node_id,
  };
  const queryClient = createQueryClient();

  render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTestProvider>
        <WorkflowActivityBentoCard
          headerAction={headerAction}
          importController={createImportController()}
          locale={locale}
          now={Date.parse("2026-04-08T12:00:04Z")}
          selection={selection}
          snapshot={snapshot}
          widgetInstanceID={widgetInstanceID}
          onSelectWorkID={vi.fn()}
          onSelectStateNode={vi.fn()}
          onSelectDoc={vi.fn()}
          onSelectResource={vi.fn()}
          onSelectWorker={vi.fn()}
          onSelectWorkType={vi.fn()}
          onSelectWorkstation={vi.fn()}
        />
      </DashboardSessionTestProvider>
    </QueryClientProvider>,
  );
}

function renderDuplicateWorkflowActivityBentoCards(locale = "zh-CN") {
  const queryClient = createQueryClient();
  const snapshot = semanticWorkflowDashboardSnapshot;
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
  const selection: DashboardSelection = {
    kind: "node",
    nodeId: selectedNode.node_id,
  };

  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTestProvider>
        <div>
          <WorkflowActivityBentoCard
            importController={createImportController()}
            locale={locale}
            now={Date.parse("2026-04-08T12:00:04Z")}
            selection={selection}
            snapshot={snapshot}
            widgetInstanceID="work-graph::primary"
            onSelectWorkID={vi.fn()}
            onSelectStateNode={vi.fn()}
            onSelectDoc={vi.fn()}
            onSelectResource={vi.fn()}
            onSelectWorker={vi.fn()}
            onSelectWorkType={vi.fn()}
            onSelectWorkstation={vi.fn()}
          />
          <WorkflowActivityBentoCard
            importController={createImportController()}
            locale={locale}
            now={Date.parse("2026-04-08T12:00:04Z")}
            selection={selection}
            snapshot={snapshot}
            widgetInstanceID="work-graph::instance-1"
            onSelectWorkID={vi.fn()}
            onSelectStateNode={vi.fn()}
            onSelectDoc={vi.fn()}
            onSelectResource={vi.fn()}
            onSelectWorker={vi.fn()}
            onSelectWorkType={vi.fn()}
            onSelectWorkstation={vi.fn()}
          />
        </div>
      </DashboardSessionTestProvider>
    </QueryClientProvider>,
  );
}

function registerWorkflowActivityBentoCardTestSetup() {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryDocumentSave).mockReturnValue({
      error: null,
      isPending: false,
      reset: vi.fn(),
      save: vi.fn(),
      saveAsync: vi.fn(),
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
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: bento card layout regressions stay grouped for header, graph shell, and editor entry.
describe("WorkflowActivityBentoCard", () => {
  registerWorkflowActivityBentoCardTestSetup();

  it("keeps flex-safe layout classes on the graph panel shell around the viewport", async () => {
    const locale = "zh-CN";
    const messages = getWorkflowActivityShellMessages(locale);
    renderWorkflowActivityBentoCard({ locale });

    const viewport = await screen.findByRole("region", {
      name: messages.viewportLabel,
    });
    const graphCard = screen.getByRole("article", {
      name: messages.widgetTitle,
    });
    const graphBody = graphCard.querySelector(
      "[data-workflow-activity-graph-body]",
    );
    const workflowSection = viewport.closest<HTMLElement>(
      "section[aria-labelledby]",
    );
    const graphPanelShell = workflowSection?.parentElement;

    expect(graphCard.className).toContain("h-full");
    expect(graphCard.className).toContain("max-h-full");
    expect(graphCard.className).toContain("min-h-0");
    expect(graphCard.className).toContain("overflow-hidden");
    expect(graphCard.style.height).toBe("100%");
    expect(graphCard.style.maxHeight).toBe("100%");
    expect(graphCard.style.overflow).toBe("hidden");
    expect(graphBody?.className).toContain("h-full");
    expect(graphBody?.className).toContain("max-h-full");
    expect(graphBody?.className).toContain("min-h-0");
    expect(graphBody?.className).toContain("overflow-hidden");
    expect((graphBody as HTMLElement | null)?.style.height).toBe("100%");
    expect((graphBody as HTMLElement | null)?.style.maxHeight).toBe("100%");
    expect((graphBody as HTMLElement | null)?.style.overflow).toBe("hidden");
    expect(graphPanelShell?.tagName).toBe("SECTION");
    expect(graphPanelShell?.className).toContain("relative");
    expect(graphPanelShell?.className).toContain("h-full");
    expect(graphPanelShell?.className).toContain("max-h-full");
    expect(graphPanelShell?.className).toContain("min-h-0");
    expect(graphPanelShell?.className).toContain("min-w-0");
    expect(graphPanelShell?.className).toContain("overflow-hidden");
  });

  it("wraps the shared topology without a floating inspector", async () => {
    const locale = "zh-CN";
    const messages = getWorkflowActivityShellMessages(locale);
    const editorMessages = getFactoryGraphEditorMessages(locale);
    renderWorkflowActivityBentoCard({ locale });

    expect(await screen.findByRole("heading", { name: "工厂图" })).toBeTruthy();
    const graphCard = screen.getByRole("article", { name: "工厂图" });
    const graphHeader = graphCard.querySelector("header");
    const graphViewport = screen.getByRole("region", {
      name: messages.viewportLabel,
    });

    expect(graphHeader).toBeTruthy();
    expect(graphCard.dataset.dashboardPanelShell).toBe("grid-card");
    expect(graphCard.className).toContain("shadow-af-card");
    expect(graphHeader?.className).toContain("min-h-11");
    expect(graphHeader?.className).toContain("px-3");
    expect(graphHeader?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(graphHeader?.className).toContain("cursor-grab");
    expect(
      within(graphCard).queryByRole("button", { name: "Move 工厂图" }),
    ).toBeNull();
    expect(
      within(graphHeader as HTMLElement).getByRole("button", {
        name: editorMessages.modeEnterEditor,
      }),
    ).toBeTruthy();
    expect(
      within(graphHeader as HTMLElement).getByText(
        editorMessages.modeObserve,
      ),
    ).toBeTruthy();
    expect(
      within(graphCard).queryByRole("heading", { name: "当前活动" }),
    ).toBeNull();
    expect(graphViewport).toBeTruthy();
    expect(graphViewport.className).not.toContain("shadow-af-card");
    expect(graphViewport.className).not.toContain("shadow-af-panel");
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(
      screen.queryByRole("button", { name: /collapse inspector/i }),
    ).toBeNull();
    expect(
      within(graphViewport).queryByRole("button", {
        name: editorMessages.modeEnterEditor,
      }),
    ).toBeNull();
  });

  it("keeps the header as the editor entry point before showing the loading editor toolbar", async () => {
    const user = userEvent.setup();
    const locale = "zh-CN";
    const shellMessages = getWorkflowActivityShellMessages(locale);
    const editorMessages = getFactoryGraphEditorMessages(locale);
    renderWorkflowActivityBentoCard({ locale });

    const graphCard = await screen.findByRole("article", {
      name: shellMessages.widgetTitle,
    });
    const graphHeader = graphCard.querySelector("header");

    expect(graphHeader).toBeTruthy();
    const headerScope = within(graphHeader as HTMLElement);
    expect(graphHeader?.className).toContain("min-h-11");
    const enterEditorButton = headerScope.getByRole("button", {
      name: editorMessages.modeEnterEditor,
    });
    expect(enterEditorButton).toBeTruthy();
    expect(headerScope.getByText(editorMessages.modeObserve)).toBeTruthy();
    expect(
      screen.queryByRole("region", {
        name: editorMessages.toolbarAriaLabel,
      }),
    ).toBeNull();
    expect(
      within(graphCard).queryByRole("heading", { name: shellMessages.title }),
    ).toBeNull();

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);

    await user.click(enterEditorButton);

    const toolbar = await screen.findByRole("region", {
      name: editorMessages.toolbarAriaLabel,
    });

    expect(
      within(toolbar).getByRole("button", {
        name: editorMessages.modeLeaveEditor,
      }),
    ).toBeTruthy();
  });

  it("keeps duplicate workflow activity cards on distinct accessibility ids", () => {
    const locale = "zh-CN";
    const messages = getWorkflowActivityShellMessages(locale);
    const { container } = renderDuplicateWorkflowActivityBentoCards(locale);

    const workflowSections = Array.from(
      container.querySelectorAll<HTMLElement>(
        'section[aria-labelledby^="workflow-graph-heading-"]',
      ),
    );
    const describedViewports = screen.getAllByRole("region", {
      name: messages.viewportLabel,
    });

    expect(workflowSections).toHaveLength(2);
    expect(describedViewports).toHaveLength(2);

    const headingIDs = workflowSections.map((section) =>
      section.getAttribute("aria-labelledby"),
    );
    expect(new Set(headingIDs).size).toBe(2);

    for (const headingID of headingIDs) {
      expect(headingID).toBeTruthy();
      expect(
        headingID ? container.ownerDocument.getElementById(headingID) : null,
      ).toBeTruthy();
    }
  });
});

describe("WorkflowActivityBentoCard header actions", () => {
  registerWorkflowActivityBentoCardTestSetup();

  it("keeps the remove action in the header without editor chrome beside it", async () => {
    const locale = "zh-CN";
    const shellMessages = getWorkflowActivityShellMessages(locale);
    const editorMessages = getFactoryGraphEditorMessages(locale);
    renderWorkflowActivityBentoCard({
      headerAction: <button type="button">Remove card</button>,
      locale,
    });

    const graphCard = await screen.findByRole("article", {
      name: shellMessages.widgetTitle,
    });
    const graphHeader = graphCard.querySelector("header");

    expect(graphHeader).toBeTruthy();
    expect(
      within(graphHeader as HTMLElement).getByRole("button", {
        name: "Remove card",
      }),
    ).toBeTruthy();
    expect(
      within(graphHeader as HTMLElement).getByRole("button", {
        name: editorMessages.modeEnterEditor,
      }),
    ).toBeTruthy();
  });

  it("shows unsaved edit chrome in the toolbar instead of the compact bento header", async () => {
    const locale = "en";
    const shellMessages = getWorkflowActivityShellMessages(locale);
    const editorMessages = getFactoryGraphEditorMessages(locale);
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        additions: {
          ...defaultDraftState.draft.additions,
          workers: [
            {
              model: "gpt-5-mini",
              name: "reviewer",
              type: "MODEL_WORKER",
            },
          ],
        },
      },
      hasChanges: true,
    } as never);

    const user = userEvent.setup();
    renderWorkflowActivityBentoCard({ locale });

    const graphCard = await screen.findByRole("article", {
      name: shellMessages.widgetTitle,
    });
    const graphHeader = graphCard.querySelector("header");
    expect(graphHeader).toBeTruthy();

    await user.click(
      within(graphHeader as HTMLElement).getByRole("button", {
        name: editorMessages.modeEnterEditor,
      }),
    );

    const toolbar = await screen.findByRole("region", {
      name: editorMessages.toolbarAriaLabel,
    });

    const headerScope = within(graphHeader as HTMLElement);
    expect(headerScope.queryAllByRole("status")).toHaveLength(0);
    expect(
      headerScope.getByText(
        editorMessages.dirtyStateSummary({
          layoutDirty: false,
          preferencesDirty: false,
          topologyDirty: true,
        }),
      ),
    ).toBeTruthy();

    const toggle = within(toolbar).getByRole("button", {
      name: editorMessages.modeLeaveEditor,
    });
    expect(toggle.className).toContain("border-af-warning-border");
    expect(toggle.className).toContain("bg-warning-container");
    expect(toggle.className).toContain("text-on-warning-container");
    expect(within(toolbar).queryAllByRole("status")).toHaveLength(0);
  });
});
