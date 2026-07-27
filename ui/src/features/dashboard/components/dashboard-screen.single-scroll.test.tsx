import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach } from "vitest";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { dashboardSemanticSnapshotFixtures } from "../../../components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { createMaterializedWorkOutcomeState } from "../../work-outcome/public/materializer";
import { DashboardScreen } from "./dashboard-screen";

const VIEWPORT_HEIGHT = 768;
const DOCUMENT_HEIGHT = 2600;
const dashboardSnapshotState = vi.hoisted(() => ({
  value: {
    error: null as Error | null,
    isInitialLoading: false,
    preflightRecovery: null as {
      reasonCode: string;
      requestedSessionId: string;
    } | null,
    preflightStatus: "success" as const,
    snapshot: null as DashboardSnapshot | null,
    streamState: {
      message: "Factory event stream connected.",
      status: "live" as const,
    },
  },
}));

let restoreBrowserShims: (() => void) | undefined;
let restoreScrollMetrics: (() => void) | undefined;

vi.mock("../../header/components/dashboard-export-dialog", () => ({
  DashboardExportDialog: () => null,
}));

vi.mock("../../header/components/dashboard-header", () => ({
  DashboardHeader: () => <header>Dashboard header</header>,
}));

vi.mock("../../header/components/dashboard-status-panel", () => ({
  DashboardStatusPanel: ({ title }: { title: string }) => (
    <section>
      <h1>{title}</h1>
    </section>
  ),
}));

vi.mock("../hooks/useDashboardSnapshot", () => ({
  useDashboardSnapshot: vi.fn(() => dashboardSnapshotState.value),
}));

vi.mock("../session/dashboard-session-provider", () => ({
  DashboardSessionProvider: ({ children }: { children: ReactNode }) => children,
  useDashboardSession: () => ({ rawSessionID: "session-test" }),
}));

vi.mock("../../bento/hooks/use-dashboard-bento-snapshot", () => ({
  useDashboardBentoSnapshot: vi.fn(() => {
    const snapshot = dashboardSnapshotState.value.snapshot;

    if (!snapshot) {
      throw new Error("Expected a dashboard snapshot fixture for bento tests.");
    }

    return {
      currentSelection: {
        canRedoSelection: false,
        canUndoSelection: false,
        clearSelectedWorkerIfMatching: vi.fn(),
        completedWorkItems: [],
        failedWorkItems: [],
        openTerminalWorkDetail: vi.fn(),
        redoSelection: vi.fn(),
        selectedNode: null,
        selectedNodeActiveExecutions: [],
        selectedNodeProviderSessions: [],
        selectedNodeWorkstationRequests: [],
        selectedStateCurrentWorkItems: [],
        selectedStatePlace: null,
        selectedStateTerminalHistoryWorkItems: [],
        selectedStateTokenCount: 0,
        selectedWorkDispatchAttempts: [],
        selectedWorkID: null,
        selectedWorkProviderSessions: [],
        selectedWorkRequestHistory: [],
        selectedWorkWorkstationRequests: [],
        selectedWorkstationRequest: null,
        selection: null,
        selectStateNode: vi.fn(),
        selectStateWorkItem: vi.fn(),
        selectWorkByID: vi.fn(),
        selectWorkItem: vi.fn(),
        selectWorkstation: vi.fn(),
        selectWorkstationRequest: vi.fn(),
        terminalWorkDetail: null,
        undoSelection: vi.fn(),
      },
      materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
      selectedSnapshot: snapshot,
      selectedTimelineTick: snapshot.tick_count,
      snapshot,
    };
  }),
}));

vi.mock("../../bento/hooks/useDashboardLayout", async () => {
  const actual = await vi.importActual<
    typeof import("../../bento/hooks/useDashboardLayout")
  >("../../bento/hooks/useDashboardLayout");

  return {
    ...actual,
    useDashboardLayout: () => ({
      addDashboardWidget: vi.fn(),
      dashboardLayout: actual.DEFAULT_DASHBOARD_LAYOUT,
      persistDashboardLayout: vi.fn(),
      removeDashboardWidget: vi.fn(),
    }),
  };
});

vi.mock("../../bento/hooks/useDashboardNow", () => ({
  useDashboardNow: () => 0,
}));

vi.mock(
  "../../current-selection/hooks/core/useCurrentSelectionDetails",
  () => ({
    useCurrentSelectionDetails: () => ({
      selectedWorkExecutionDetails: null,
      selectedWorkRelationshipGraph: { status: "empty" as const },
    }),
  }),
);

vi.mock(
  "../../current-selection/work-selection/hooks/useSelectedProviderSessionState",
  () => ({
    useSelectedProviderSessionState: () => ({
      selectedProviderSession: null,
      selectedProviderSessionKey: null,
      setSelectedProviderSession: vi.fn(),
    }),
  }),
);

vi.mock("../../trace-drilldown/hooks/useTraceDrilldown", () => ({
  useTraceDrilldown: () => ({
    selectedTrace: null,
    traceGridState: { status: "empty" as const },
  }),
}));

vi.mock("../../work-outcome/hooks/useWorkOutcomeChart", () => ({
  useWorkOutcomeChart: () => ({ status: "empty" as const }),
}));

vi.mock(
  "../../workflow-activity/hooks/current-activity-import-controller",
  () => ({
    useCurrentActivityImportController: () => ({
      activationState: { status: "idle" as const },
      activateImport: vi.fn(),
      clearActivationError: vi.fn(),
      closeImportPreview: vi.fn(),
      importPreviewState: { status: "idle" as const },
      openImportPreview: vi.fn(),
    }),
  }),
);

vi.mock("../../import/components/dashboard-import-preview-dialog", () => ({
  DashboardImportPreviewDialog: () => null,
}));

vi.mock("../../current-selection/components/widget/current-selection-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    CurrentSelectionWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Current selection"
        widgetId="current-selection"
      >
        <p>Current selection fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

vi.mock("../../provider-session-detail/components/provider-session-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    ProviderSessionWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Provider session"
        widgetId="provider-session"
      >
        <p>Provider session fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

vi.mock("../../submit-work/components/submit-work-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    SubmitWorkWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Submit work"
        widgetId="submit-work"
      >
        <p>Submit work fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

vi.mock("../../terminal-work/components/terminal-work-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    TerminalWorkWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Terminal work"
        widgetId="terminal-work"
      >
        <p>Terminal work fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

vi.mock("../../trace-drilldown/components/trace-drilldown-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    TraceDrilldownWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Trace drilldown"
        widgetId="trace-drilldown"
      >
        <p>Trace fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

vi.mock("../../work-outcome/components/work-outcome-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    WorkOutcomeWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Work outcome"
        widgetId="work-outcome"
      >
        <p>Work outcome fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

vi.mock("../../work-totals/components/work-totals-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    WorkTotalsWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Work totals"
        widgetId="work-totals"
      >
        <p>Work totals fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

vi.mock("../../workflow-activity/components/workflow-activity-widget", async () => {
  const { DashboardWidgetFrame } = await vi.importActual<
    typeof import("../../bento/components/dashboard-widget-frame/dashboard-widget-frame")
  >("../../bento/components/dashboard-widget-frame/dashboard-widget-frame");

  return {
    WorkflowActivityWidget: ({
      headerAction,
    }: {
      headerAction?: React.ReactNode;
    }) => (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title="Workflow activity"
        widgetId="workflow-activity"
      >
        <p>Workflow activity fixture content</p>
      </DashboardWidgetFrame>
    ),
  };
});

function setElementMetric(
  element: Element,
  property: "clientHeight" | "scrollHeight",
  value: number,
) {
  const descriptor = Object.getOwnPropertyDescriptor(element, property);

  Object.defineProperty(element, property, {
    configurable: true,
    value,
  });

  return () => {
    if (descriptor) {
      Object.defineProperty(element, property, descriptor);
    } else {
      Reflect.deleteProperty(element, property);
    }
  };
}

function installScrollMetricFixture(dashboardRoot: HTMLElement) {
  const restoreFns = [
    setElementMetric(document.documentElement, "clientHeight", VIEWPORT_HEIGHT),
    setElementMetric(document.documentElement, "scrollHeight", DOCUMENT_HEIGHT),
    setElementMetric(document.body, "clientHeight", DOCUMENT_HEIGHT),
    setElementMetric(document.body, "scrollHeight", DOCUMENT_HEIGHT),
    setElementMetric(dashboardRoot, "clientHeight", VIEWPORT_HEIGHT),
    setElementMetric(dashboardRoot, "scrollHeight", DOCUMENT_HEIGHT),
  ];

  return () => {
    for (const restore of restoreFns.reverse()) {
      restore();
    }
  };
}

function isVerticalScrollOwner(element: HTMLElement) {
  const overflowY = window.getComputedStyle(element).overflowY;
  const isDocumentElement = element === document.documentElement;
  const permitsScrolling =
    isDocumentElement || overflowY === "auto" || overflowY === "scroll";

  return element.scrollHeight > element.clientHeight && permitsScrolling;
}

function getVerticalScrollOwners(elements: HTMLElement[]) {
  return elements.filter(isVerticalScrollOwner);
}

describe("DashboardScreen scroll ownership", () => {
  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
    dashboardSnapshotState.value = {
      error: null,
      isInitialLoading: false,
      snapshot: dashboardSemanticSnapshotFixtures.activeWork,
      streamState: {
        message: "Factory event stream connected.",
        status: "live",
      },
    };
  });

  afterEach(() => {
    restoreScrollMetrics?.();
    restoreScrollMetrics = undefined;
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders a tall loaded dashboard while preserving shared widget scrollports", () => {
    render(<DashboardScreen />);

    const dashboardRoot = screen.getByRole("main");
    const board = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const grid = board.querySelector(".react-grid-layout");

    if (!(grid instanceof HTMLElement)) {
      throw new Error("Expected the loaded dashboard to render a bento grid.");
    }

    expect(Number.parseFloat(grid.style.height)).toBeGreaterThan(
      VIEWPORT_HEIGHT,
    );
    restoreScrollMetrics = installScrollMetricFixture(dashboardRoot);

    expect(
      getVerticalScrollOwners([
        document.documentElement,
        document.body,
        dashboardRoot,
      ]),
    ).toEqual([document.documentElement]);
    expect(
      dashboardRoot.querySelector("[data-radix-scroll-area-viewport]"),
    ).toBeTruthy();
  });
});
