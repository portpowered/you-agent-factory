import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} from "../../current-factory-definition/public";
import type { DashboardSelection } from "../../current-selection/public";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { useFactoryGraphDraftState } from "../../factory-graph-editor/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

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

vi.mock("../../factory-graph-editor/public", async () => {
  const actual = await vi.importActual("../../factory-graph-editor/public");

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
        onSelectWorker={vi.fn()}
        onSelectWorkstation={vi.fn()}
      />
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
          onSelectWorker={vi.fn()}
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
          onSelectWorker={vi.fn()}
          onSelectWorkstation={vi.fn()}
        />
      </div>
    </QueryClientProvider>,
  );
}

function registerWorkflowActivityBentoCardTestSetup() {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);
    vi.mocked(useSaveCurrentFactory).mockReturnValue({
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
}

describe("WorkflowActivityBentoCard", () => {
  registerWorkflowActivityBentoCardTestSetup();

  it("wraps the React Flow graph without a floating inspector", async () => {
    const locale = "zh-CN";
    const messages = getWorkflowActivityShellMessages(locale);
    renderWorkflowActivityBentoCard({ locale });

    expect(await screen.findByRole("heading", { name: "工厂图" })).toBeTruthy();
    const graphCard = screen.getByRole("article", { name: "工厂图" });
    const graphHeader = graphCard.querySelector("header");

    expect(graphHeader).toBeTruthy();
    expect(graphHeader?.className).toContain("min-h-11");
    expect(graphHeader?.className).toContain("px-3");
    expect(
      within(graphCard).getByRole("button", { name: "进入工厂图编辑器" }),
    ).toBeTruthy();
    expect(
      within(graphCard).getByRole("button", { name: "Move 工厂图" }).className,
    ).toContain("h-10");
    expect(within(graphCard).getByText("观察模式")).toBeTruthy();
    expect(graphHeader?.textContent).toContain("观察模式");
    expect(
      within(graphCard).queryByRole("heading", { name: "当前活动" }),
    ).toBeNull();
    expect(
      screen.getByRole("region", { name: messages.viewportLabel }),
    ).toBeTruthy();
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(
      screen.queryByRole("button", { name: /collapse inspector/i }),
    ).toBeNull();
  });

  it("keeps the header as the visible editor entry point in observe and loading editor states", async () => {
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
    expect(headerScope.getByText(editorMessages.modeObserve)).toBeTruthy();
    expect(
      headerScope.getByRole("button", { name: editorMessages.modeEnterEditor })
        .className,
    ).toContain("size-8");
    expect(
      within(graphCard).queryByRole("heading", { name: shellMessages.title }),
    ).toBeNull();

    await user.click(
      headerScope.getByRole("button", { name: editorMessages.modeEnterEditor }),
    );

    expect(
      headerScope.getByText(editorMessages.modeLoadingDefinition),
    ).toBeTruthy();
    expect(
      headerScope.getByRole("button", { name: editorMessages.modeLeaveEditor }),
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
    const viewportDescriptionIDs = describedViewports.map((region) =>
      region.getAttribute("aria-describedby"),
    );

    expect(new Set(headingIDs).size).toBe(2);
    expect(new Set(viewportDescriptionIDs).size).toBe(2);
    expect(headingIDs).toEqual(viewportDescriptionIDs);

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

  it("orders the remove action with the graph header controls instead of before the status pill", async () => {
    const locale = "zh-CN";
    const shellMessages = getWorkflowActivityShellMessages(locale);
    renderWorkflowActivityBentoCard({
      headerAction: <button type="button">Remove card</button>,
      locale,
    });

    const graphCard = await screen.findByRole("article", {
      name: shellMessages.widgetTitle,
    });
    const actionSections = graphCard.querySelectorAll(
      "[data-dashboard-action-row-section]",
    );

    expect(actionSections).toHaveLength(2);
    expect(
      actionSections[0]?.getAttribute("data-dashboard-action-row-section"),
    ).toBe("statuses");
    expect(
      actionSections[1]?.getAttribute("data-dashboard-action-row-section"),
    ).toBe("actions");

    const actions = within(actionSections[1] as HTMLElement).getAllByRole(
      "button",
    );

    expect(actions[0]?.getAttribute("aria-label")).toBe("进入工厂图编辑器");
    expect(actions[1]?.textContent).toBe("Remove card");
  });
});
