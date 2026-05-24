import { fireEvent, render, screen } from "@testing-library/react";
import type { LoadableProviderSessionRef } from "../../provider-session-detail/public";

import { DEFAULT_DASHBOARD_LAYOUT } from "../hooks/dashboardLayoutSchema";

import { DashboardBento } from "./dashboard-bento";

const addDashboardWidget = vi.fn();
const removeDashboardWidget = vi.fn();

const currentSelectionState = {
  canRedoSelection: false,
  canUndoSelection: false,
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
};

const SHARED_SELECTED_SESSION: LoadableProviderSessionRef = {
  dispatchID: "dispatch-review-active",
  id: "sess-shared",
  kind: "session_id",
  provider: "codex",
};

vi.mock("../../current-selection/public", async () => {
  const React = await vi.importActual<typeof import("react")>("react");

  return {
    CurrentSelectionWidget: ({
      headerAction,
      onSelectProviderSession,
    }: {
      headerAction?: React.ReactNode;
      onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
    }) => (
      <section>
        {headerAction}
        <p>Current selection card</p>
        <button
          onClick={() => onSelectProviderSession?.(SHARED_SELECTED_SESSION)}
          type="button"
        >
          Select shared provider session
        </button>
      </section>
    ),
    useCurrentSelection: () => currentSelectionState,
    useCurrentSelectionDetails: () => ({
      selectedWorkExecutionDetails: null,
    }),
    useSelectedProviderSessionState: () => {
      const [selectedProviderSession, setSelectedProviderSession] =
        React.useState<LoadableProviderSessionRef | null>(null);

      return {
        selectedProviderSession,
        selectedProviderSessionKey: selectedProviderSession?.id ?? null,
        setSelectedProviderSession,
      };
    },
  };
});

vi.mock("../../provider-session-detail/public", () => ({
  ProviderSessionWidget: ({
    headerAction,
    selectedProviderSession,
  }: {
    headerAction?: React.ReactNode;
    selectedProviderSession: LoadableProviderSessionRef | null;
  }) => (
    <section>
      {headerAction}
      Provider session card
      {selectedProviderSession ? `: ${selectedProviderSession.id}` : ""}
    </section>
  ),
}));

vi.mock("../../import", () => ({
  DashboardImportPreviewDialog: () => null,
}));

vi.mock("../../submit-work/public", () => ({
  SubmitWorkWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Submit work card</section>,
}));

vi.mock("../../terminal-work/public", () => ({
  TerminalWorkWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Terminal work card</section>,
}));

vi.mock("../../dashboard/state/dashboardSessionStore", () => ({
  useDashboardSessionStore: (selector: (state: { selectedSessionID: string }) => unknown) =>
    selector({ selectedSessionID: "session-1" }),
}));

vi.mock("../../timeline/state/factoryTimelineStore", () => ({
  useFactoryTimelineStore: (
    selector: (state: {
      events: [];
      selectedTick: number;
      worldViewCache: Record<number, unknown>;
    }) => unknown,
  ) =>
    selector({
      events: [],
      selectedTick: 1,
      worldViewCache: {
        1: {
          runtime: {
            active_workstation_node_ids: [],
            in_flight_dispatch_count: 0,
            session: {
              completed_count: 0,
              dispatched_count: 0,
              failed_count: 0,
              has_data: true,
            },
            workstation_requests_by_dispatch_id: {},
          },
          tick_count: 1,
          topology: {
            edges: [],
            submit_work_types: [],
            workstation_node_ids: [],
            workstation_nodes_by_id: {},
          },
          uptime_seconds: 0,
          workstationRequestsByDispatchID: {},
        },
      },
    }),
}));

vi.mock("../../trace-drilldown/public", () => ({
  TraceDrilldownWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Trace card</section>,
  useTraceDrilldown: () => ({
    selectedTrace: null,
    traceGridState: { status: "empty" },
  }),
}));

vi.mock("../../work-outcome/public", () => ({
  WorkOutcomeWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Work outcome card</section>,
  useWorkOutcomeChart: () => ({ status: "empty" }),
}));

vi.mock("../../work-totals/public", () => ({
  WorkTotalsWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Work totals card</section>,
}));

vi.mock("../../workflow-activity/public", () => ({
  WorkflowActivityWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Workflow activity card</section>,
  useCurrentActivityImportController: () => ({
    activationState: { status: "idle" },
    activateImport: vi.fn(),
    clearActivationError: vi.fn(),
    closeImportPreview: vi.fn(),
    importPreviewState: { status: "idle" },
  }),
}));

vi.mock("../state/dashboardBentoStore", () => ({
  useDashboardBentoStore: (
    selector: (state: {
      incrementRefreshToken: () => void;
      refreshToken: number;
      resetSelectedTraceID: () => void;
      selectedTraceID: string | null;
      setSelectedTraceID: (traceID: string | null) => void;
    }) => unknown,
  ) =>
    selector({
      incrementRefreshToken: vi.fn(),
      refreshToken: 0,
      resetSelectedTraceID: vi.fn(),
      selectedTraceID: null,
      setSelectedTraceID: vi.fn(),
    }),
}));

vi.mock("../hooks/useDashboardLayout", () => ({
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID: "add-widget::inline-add",
  DASHBOARD_WIDGET_IDS: {
    addWidget: "add-widget",
    currentSelection: "current-selection",
    providerSession: "provider-session",
    submitWork: "submit-work",
    terminalWork: "terminal-work",
    trace: "trace",
    workGraph: "work-graph",
    workOutcomeChart: "work-outcome-chart",
    workTotals: "work-totals",
  },
  getRenderableDashboardLayout: (layout: unknown) => layout,
  useDashboardLayout: () => ({
    addDashboardWidget,
    dashboardLayout: DEFAULT_DASHBOARD_LAYOUT,
    persistDashboardLayout: vi.fn(),
    removeDashboardWidget,
  }),
}));

vi.mock("../hooks/useDashboardNow", () => ({
  useDashboardNow: () => 0,
}));

vi.mock("./agent-bento", () => ({
  AgentBentoLayout: ({
    cards,
  }: {
    cards: Array<{ children: React.ReactNode; id: string; widgetType?: string }>;
  }) => (
    <div>
      {cards.map((card) => (
        <div data-testid={card.widgetType ?? card.id} key={card.id}>
          {card.children}
        </div>
      ))}
    </div>
  ),
}));

vi.mock("./inline-add-widget-card", () => ({
  InlineAddWidgetCard: ({
    onSelectWidget,
  }: {
    onSelectWidget?: (widgetType: string) => void;
  }) => (
    <section>
      Add widget card
      <button onClick={() => onSelectWidget?.("work-graph")} type="button">
        Add workflow activity widget
      </button>
    </section>
  ),
}));

describe("DashboardBento", () => {
  beforeEach(() => {
    addDashboardWidget.mockReset();
    removeDashboardWidget.mockReset();
  });

  it("registers the provider-session card alongside current selection", () => {
    render(<DashboardBento />);

    expect(screen.getByTestId("current-selection").textContent).toContain(
      "Current selection card",
    );
    expect(screen.getByTestId("provider-session").textContent).toContain(
      "Provider session card",
    );
    expect(screen.getByTestId("add-widget").textContent).toContain(
      "Add widget card",
    );
  });

  it("keeps provider-session selection centralized between the current selection and provider-session cards", () => {
    render(<DashboardBento />);

    expect(screen.getByTestId("provider-session").textContent).not.toContain(
      SHARED_SELECTED_SESSION.id,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Select shared provider session" }),
    );

    expect(screen.getByTestId("provider-session").textContent).toContain(
      SHARED_SELECTED_SESSION.id,
    );
  });

  it("adds a widget instance through the inline picker selection seam", () => {
    render(<DashboardBento />);

    fireEvent.click(
      screen.getByRole("button", { name: "Add workflow activity widget" }),
    );

    expect(addDashboardWidget).toHaveBeenCalledWith("work-graph");
  });

  it("renders compact remove controls for removable dashboard cards and routes removal by instance id", () => {
    render(<DashboardBento />);

    const workTotalsRemoveButton = screen.getByRole("button", {
      name: "Remove Work totals widget from dashboard",
    });

    expect(workTotalsRemoveButton.className).toContain("size-8");
    expect(workTotalsRemoveButton.className).toContain(
      "focus-visible:outline-2",
    );
    expect(
      screen.queryByRole("button", {
        name: "Remove Add widget widget from dashboard",
      }),
    ).toBeNull();

    fireEvent.click(workTotalsRemoveButton);

    expect(removeDashboardWidget).toHaveBeenCalledWith("work-totals::primary");
  });
});
