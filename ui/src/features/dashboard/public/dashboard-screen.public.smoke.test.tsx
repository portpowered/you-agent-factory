import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";

import { useAppLocale } from "../../../i18n";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { useDashboardSession } from "../session/dashboard-session-provider";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../state/dashboardSessionStore";
import { DashboardScreen } from "./index";

const EXPECTED_DASHBOARD_SHELL_CLASS =
  "min-h-screen overflow-x-hidden p-1 md:p-2";

function renderDashboardScreen() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardScreen />
    </QueryClientProvider>,
  );
}

let dashboardSnapshotState: ReturnType<
  typeof import("../hooks/useDashboardSnapshot").useDashboardSnapshot
>;

vi.mock("../../bento/components/dashboard-bento", () => ({
  DashboardBento: ({ locale }: { locale?: string }) => {
    const { locale: resolvedLocale } = useAppLocale(locale);
    return <section>Dashboard bento {resolvedLocale}</section>;
  },
}));

vi.mock("../../header/components/dashboard-export-dialog", () => ({
  DashboardExportDialog: ({ locale }: { locale?: string }) => {
    const { locale: resolvedLocale } = useAppLocale(locale);
    return <div>Dashboard export dialog {resolvedLocale}</div>;
  },
}));

vi.mock("../../header/components/dashboard-header", () => ({
  DashboardHeader: ({ locale }: { locale?: string }) => {
    const { locale: resolvedLocale } = useAppLocale(locale);
    const { sessionID } = useDashboardSession();
    return (
      <header>
        Dashboard header {sessionID} {resolvedLocale}
      </header>
    );
  },
}));

vi.mock("../../header/components/dashboard-status-panel", () => ({
  DashboardStatusPanel: ({
    detail,
    title,
  }: {
    detail?: string;
    title: string;
  }) => (
    <section>
      <h1>{title}</h1>
      {detail ? <p>{detail}</p> : null}
    </section>
  ),
}));

vi.mock("../hooks/useDashboardSnapshot", () => ({
  useDashboardSnapshot: vi.fn(() => dashboardSnapshotState),
}));

describe("dashboard public barrel composition", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: "session-review",
    });
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: true,
      preflightRecovery: null,
      preflightStatus: "loading",
      snapshot: null,
      streamState: {
        message: "Loading factory events...",
        status: "connecting",
      },
    };
  });

  it("renders DashboardScreen from the public barrel with session-scoped loading shell", () => {
    const messages = getHeaderControlsMessages("en");

    renderDashboardScreen();

    expect(screen.getByRole("main").className).toBe(
      EXPECTED_DASHBOARD_SHELL_CLASS,
    );
    expect(
      screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
    ).toBeTruthy();
  });

  it("keeps active session scope when the public barrel renders dashboard content", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      preflightRecovery: null,
      preflightStatus: "success",
      snapshot: {
        factory_state: "RUNNING",
        runtime: {
          session: {
            bracket: {
              result_status: "IN_PROGRESS",
              started_at: "2026-06-09T12:00:00Z",
            },
          },
        },
      } as never,
      streamState: {
        message: "Factory event stream connected.",
        status: "live",
      },
    };

    renderDashboardScreen();

    expect(screen.getByRole("main").className).toBe(
      EXPECTED_DASHBOARD_SHELL_CLASS,
    );
    expect(
      screen.queryByTestId("dashboard-session-lifecycle-banner"),
    ).toBeNull();
    expect(screen.getByText("Dashboard header session-review en")).toBeTruthy();
    expect(screen.getByText("Dashboard bento en")).toBeTruthy();
    expect(screen.getByText("Dashboard export dialog en")).toBeTruthy();
  });
});
