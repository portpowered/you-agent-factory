import { render, screen } from "@testing-library/react";

import type { DashboardSnapshot } from "../../api/dashboard/types";
import { DashboardScreen } from "./dashboard-screen";

const mockDashboardHeader = vi.fn(({ locale }: { locale?: string }) => (
  <div data-testid="dashboard-header-locale">{locale}</div>
));
const mockDashboardBento = vi.fn(({ locale }: { locale?: string }) => (
  <div data-testid="dashboard-bento-locale">{locale}</div>
));
const mockDashboardExportDialog = vi.fn(({ locale }: { locale?: string }) => (
  <div data-testid="dashboard-export-locale">{locale}</div>
));
const mockUseDashboardSnapshot = vi.fn();

vi.mock("../header", () => ({
  DashboardExportDialog: (props: { locale?: string }) => mockDashboardExportDialog(props),
  DashboardHeader: (props: { locale?: string }) => mockDashboardHeader(props),
  DashboardStatusPanel: ({ detail, title }: { detail?: string; title: string }) => (
    <div data-testid="dashboard-status-panel">
      <span>{title}</span>
      {detail ? <span>{detail}</span> : null}
    </div>
  ),
}));

vi.mock("../bento", () => ({
  DashboardBento: (props: { locale?: string }) => mockDashboardBento(props),
}));

vi.mock("./useDashboardSnapshot", () => ({
  useDashboardSnapshot: () => mockUseDashboardSnapshot(),
}));

const READY_SNAPSHOT = {
  factory_state: "IDLE",
  runtime: {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: true,
    },
  },
  tick_count: 1,
  topology: {
    edges: [],
    submit_work_types: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  },
  uptime_seconds: 0,
} satisfies DashboardSnapshot;

describe("DashboardScreen", () => {
  beforeEach(() => {
    document.documentElement.lang = "";
    mockDashboardHeader.mockClear();
    mockDashboardBento.mockClear();
    mockDashboardExportDialog.mockClear();
    mockUseDashboardSnapshot.mockReset();
  });

  it("passes the resolved document locale through the production dashboard composition", () => {
    document.documentElement.lang = "ja-JP";
    mockUseDashboardSnapshot.mockReturnValue({
      error: null,
      isInitialLoading: false,
      snapshot: READY_SNAPSHOT,
    });

    render(<DashboardScreen />);

    expect(screen.getByTestId("dashboard-header-locale").textContent).toBe("ja");
    expect(screen.getByTestId("dashboard-bento-locale").textContent).toBe("ja");
    expect(screen.getByTestId("dashboard-export-locale").textContent).toBe("ja");
  });

  it("renders the loading status panel while the initial dashboard snapshot is loading", () => {
    mockUseDashboardSnapshot.mockReturnValue({
      error: null,
      isInitialLoading: true,
      snapshot: null,
    });

    render(<DashboardScreen />);

    expect(screen.getByTestId("dashboard-status-panel").textContent).toContain(
      "Loading dashboard",
    );
  });

  it("renders the error status panel when the dashboard snapshot fails", () => {
    mockUseDashboardSnapshot.mockReturnValue({
      error: new Error("Network unreachable"),
      isInitialLoading: false,
      snapshot: null,
    });

    render(<DashboardScreen />);

    expect(screen.getByTestId("dashboard-status-panel").textContent).toContain(
      "Dashboard unavailable",
    );
    expect(screen.getByTestId("dashboard-status-panel").textContent).toContain(
      "Network unreachable",
    );
  });
});
