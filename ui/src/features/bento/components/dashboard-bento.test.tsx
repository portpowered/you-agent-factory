import { fireEvent, render, screen } from "@testing-library/react";
import type { LoadableProviderSessionRef } from "../../provider-session-detail/public";

import { DashboardBento } from "./dashboard-bento";

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
      onSelectProviderSession,
    }: {
      onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
    }) => (
      <section>
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
    selectedProviderSession,
  }: {
    selectedProviderSession: LoadableProviderSessionRef | null;
  }) => (
    <section>
      Provider session card
      {selectedProviderSession ? `: ${selectedProviderSession.id}` : ""}
    </section>
  ),
}));

vi.mock("../../import", () => ({
  DashboardImportPreviewDialog: () => null,
}));

vi.mock("../../submit-work/public", () => ({
  SubmitWorkWidget: () => <section>Submit work card</section>,
}));

vi.mock("../../terminal-work", () => ({
  TerminalWorkWidget: () => <section>Terminal work card</section>,
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

vi.mock("../../trace-drilldown", () => ({
  TraceDrilldownWidget: () => <section>Trace card</section>,
  useTraceDrilldown: () => ({
    selectedTrace: null,
    traceGridState: { status: "empty" },
  }),
}));

vi.mock("../../work-outcome", () => ({
  WorkOutcomeWidget: () => <section>Work outcome card</section>,
  useWorkOutcomeChart: () => ({ status: "empty" }),
}));

vi.mock("../../work-totals/public", () => ({
  WorkTotalsWidget: () => <section>Work totals card</section>,
}));

vi.mock("../../workflow-activity", () => ({
  WorkflowActivityWidget: () => <section>Workflow activity card</section>,
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
  DASHBOARD_WIDGET_IDS: {
    currentSelection: "current-selection",
    providerSession: "provider-session",
    submitWork: "submit-work",
    terminalWork: "terminal-work",
    trace: "trace",
    workGraph: "work-graph",
    workOutcomeChart: "work-outcome-chart",
    workTotals: "work-totals",
  },
  useDashboardLayout: () => ({
    dashboardLayout: [],
    persistDashboardLayout: vi.fn(),
  }),
}));

vi.mock("../hooks/useDashboardNow", () => ({
  useDashboardNow: () => 0,
}));

vi.mock("./agent-bento", () => ({
  AgentBentoLayout: ({ cards }: { cards: Array<{ id: string; children: React.ReactNode }> }) => (
    <div>
      {cards.map((card) => (
        <div data-testid={card.id} key={card.id}>
          {card.children}
        </div>
      ))}
    </div>
  ),
}));

describe("DashboardBento", () => {
  it("registers the provider-session card alongside current selection", () => {
    render(<DashboardBento />);

    expect(screen.getByTestId("current-selection").textContent).toContain(
      "Current selection card",
    );
    expect(screen.getByTestId("provider-session").textContent).toContain(
      "Provider session card",
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
});
