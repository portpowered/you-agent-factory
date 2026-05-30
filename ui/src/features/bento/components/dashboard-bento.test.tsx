import { fireEvent, render, screen } from "@testing-library/react";
import * as React from "react";
import type { LoadableProviderSessionRef } from "../../provider-session-detail/lib/provider-session-ref";

import { DEFAULT_DASHBOARD_LAYOUT } from "../hooks/dashboardLayoutSchema";

import { DashboardBento } from "./dashboard-bento";

const addDashboardWidget = vi.fn();
const removeDashboardWidget = vi.fn();
let mockDashboardLayout = DEFAULT_DASHBOARD_LAYOUT;

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

vi.mock("../../current-selection/public", () => ({
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
}));

vi.mock("../../current-selection/hooks/useCurrentSelection", () => ({
  useCurrentSelection: () => currentSelectionState,
}));

vi.mock("../../current-selection/hooks/useCurrentSelectionDetails", () => ({
  useCurrentSelectionDetails: () => ({
    selectedWorkExecutionDetails: null,
    selectedWorkRelationshipGraph: { status: "loading" as const },
  }),
}));

vi.mock(
  "../../current-selection/work-selection/hooks/useSelectedProviderSessionState",
  () => ({
    useSelectedProviderSessionState: () => {
      const [selectedProviderSession, setSelectedProviderSession] =
        React.useState<LoadableProviderSessionRef | null>(null);

      return {
        selectedProviderSession,
        selectedProviderSessionKey: selectedProviderSession?.id ?? null,
        setSelectedProviderSession,
      };
    },
  }),
);

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

vi.mock("../../import/public", () => ({
  DashboardImportPreviewDialog: () => null,
}));

vi.mock("../../import/hooks/use-factory-import-activation-target", () => ({
  useFactoryImportActivationTarget: () => ({
    createTargetFactoryName: null,
    currentFactoryName: null,
    existingNamedFactoryNames: [],
    isLoading: false,
    replacesExistingCreateTarget: false,
  }),
}));

vi.mock("../../submit-work/public", () => ({
  SubmitWorkWidget: ({ headerAction }: { headerAction?: React.ReactNode }) => (
    <section>{headerAction}Submit work card</section>
  ),
}));

vi.mock("../../terminal-work/public", () => ({
  TerminalWorkWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Terminal work card</section>,
}));

vi.mock("../../dashboard/session/dashboard-session-provider", () => ({
  useDashboardSession: () => ({
    eventsPath: "/factory-sessions/session-1/events",
    factoryPath: "/factory-sessions/session-1/factory",
    isDefault: false,
    isPaused: false,
    rawSessionID: "session-1",
    sessionID: "session-1",
    workPath: "/factory-sessions/session-1/work",
  }),
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

vi.mock("../../trace-drilldown/hooks/useTraceDrilldown", () => ({
  useTraceDrilldown: () => ({
    selectedTrace: null,
    traceGridState: { status: "empty" },
  }),
}));

vi.mock("../../trace-drilldown/public", () => ({
  TraceDrilldownWidget: ({
    headerAction,
  }: {
    headerAction?: React.ReactNode;
  }) => <section>{headerAction}Trace card</section>,
}));

vi.mock("../../work-outcome/hooks/useWorkOutcomeChart", () => ({
  useWorkOutcomeChart: () => ({ status: "empty" }),
}));

vi.mock("../../work-outcome/public", () => ({
  WorkOutcomeWidget: ({ headerAction }: { headerAction?: React.ReactNode }) => (
    <section>{headerAction}Work outcome card</section>
  ),
}));

vi.mock("../../work-totals/public", () => ({
  WorkTotalsWidget: ({ headerAction }: { headerAction?: React.ReactNode }) => (
    <section>{headerAction}Work totals card</section>
  ),
}));

vi.mock("../../workflow-activity/public", () => ({
  WorkflowActivityWidget: ({
    headerAction,
    widgetInstanceID,
  }: {
    headerAction?: React.ReactNode;
    widgetInstanceID?: string;
  }) => (
    <section>
      {headerAction}
      Workflow activity card
      {widgetInstanceID ? `:${widgetInstanceID}` : ""}
    </section>
  ),
}));

vi.mock(
  "../../workflow-activity/hooks/current-activity-import-controller",
  () => ({
    useCurrentActivityImportController: () => ({
      activationState: { status: "idle" },
      activateImport: vi.fn(),
      clearActivationError: vi.fn(),
      closeImportPreview: vi.fn(),
      importPreviewState: { status: "idle" },
    }),
  }),
);

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
  useDashboardLayout: () => {
    const [dashboardLayout, setDashboardLayout] =
      React.useState(mockDashboardLayout);

    return {
      addDashboardWidget: (widgetType: string) => {
        addDashboardWidget(widgetType);
      },
      dashboardLayout,
      persistDashboardLayout: vi.fn(),
      removeDashboardWidget: (widgetInstanceID: string) => {
        removeDashboardWidget(widgetInstanceID);
        setDashboardLayout((currentLayout) =>
          currentLayout.filter((item) => item.id !== widgetInstanceID),
        );
      },
    };
  },
}));

vi.mock("../hooks/useDashboardNow", () => ({
  useDashboardNow: () => 0,
}));

vi.mock("./agent-bento", () => ({
  AgentBentoLayout: ({
    cards,
  }: {
    cards: Array<{
      children: React.ReactNode;
      id: string;
      widgetType?: string;
    }>;
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

vi.mock("../../dashboard-add-card/components/inline-add-widget-card", () => ({
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
    mockDashboardLayout = DEFAULT_DASHBOARD_LAYOUT;
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

  it("passes stable workflow activity instance ids into duplicate-capable dashboard cards", () => {
    mockDashboardLayout = [
      ...DEFAULT_DASHBOARD_LAYOUT,
      {
        h: 8,
        id: "work-graph::instance-1",
        minH: 6,
        minW: 5,
        w: 8,
        widgetType: "work-graph",
        x: 0,
        y: 12,
      },
    ];

    render(<DashboardBento />);

    expect(
      screen.getByText("Workflow activity card:work-graph::primary"),
    ).toBeTruthy();
    expect(
      screen.getByText("Workflow activity card:work-graph::instance-1"),
    ).toBeTruthy();
  });

  it("renders compact remove controls for removable dashboard cards and routes removal by instance id", () => {
    render(<DashboardBento />);

    const workTotalsRemoveButton = screen.getByRole("button", {
      name: "Remove Work totals widget from dashboard",
    });

    expect(workTotalsRemoveButton.className).toContain("h-10");
    expect(workTotalsRemoveButton.className).toContain("w-10");
    expect(workTotalsRemoveButton.className).toContain("focus-visible:ring-2");
    expect(
      screen.queryByRole("button", {
        name: "Remove Add widget widget from dashboard",
      }),
    ).toBeNull();

    fireEvent.click(workTotalsRemoveButton);

    expect(removeDashboardWidget).toHaveBeenCalledWith("work-totals::primary");
  });

  it("removes only the targeted duplicate widget instance and keeps the inline add card", () => {
    mockDashboardLayout = [
      ...DEFAULT_DASHBOARD_LAYOUT,
      {
        h: 5,
        id: "work-outcome-chart::instance-1",
        minH: 4,
        minW: 3,
        w: 4,
        widgetType: "work-outcome-chart",
        x: 8,
        y: 10,
      },
    ];

    render(<DashboardBento />);

    const workOutcomeRemoveButtons = screen.getAllByRole("button", {
      name: "Remove Work outcome chart widget from dashboard",
    });

    expect(screen.getAllByText("Work outcome card")).toHaveLength(2);
    expect(screen.getByTestId("add-widget").textContent).toContain(
      "Add widget card",
    );

    fireEvent.click(workOutcomeRemoveButtons[1] ?? workOutcomeRemoveButtons[0]);

    expect(removeDashboardWidget).toHaveBeenCalledWith(
      "work-outcome-chart::instance-1",
    );
    expect(screen.getAllByText("Work outcome card")).toHaveLength(1);
    expect(screen.getByTestId("add-widget").textContent).toContain(
      "Add widget card",
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByText(/undo/i)).toBeNull();
  });
});
