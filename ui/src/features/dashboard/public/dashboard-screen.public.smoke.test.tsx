import { render, screen } from "@testing-library/react";

import { useAppLocale } from "../../../i18n";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { useDashboardSession } from "../session/dashboard-session-provider";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../state/dashboardSessionStore";
import { DashboardScreen } from "./index";

const EXPECTED_DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";

let dashboardSnapshotState: ReturnType<
  typeof import("../hooks/useDashboardSnapshot").useDashboardSnapshot
>;

vi.mock("../../bento/public", () => ({
  DashboardBento: ({ locale }: { locale?: string }) => {
    const { locale: resolvedLocale } = useAppLocale(locale);
    return <section>Dashboard bento {resolvedLocale}</section>;
  },
}));

vi.mock("../../header/public", () => ({
  DashboardExportDialog: ({ locale }: { locale?: string }) => {
    const { locale: resolvedLocale } = useAppLocale(locale);
    return <div>Dashboard export dialog {resolvedLocale}</div>;
  },
  DashboardHeader: ({ locale }: { locale?: string }) => {
    const { locale: resolvedLocale } = useAppLocale(locale);
    const { sessionID } = useDashboardSession();
    return (
      <header>
        Dashboard header {sessionID} {resolvedLocale}
      </header>
    );
  },
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
      snapshot: null,
    };
  });

  it("renders DashboardScreen from the public barrel with session-scoped loading shell", () => {
    const messages = getHeaderControlsMessages("en");

    render(<DashboardScreen />);

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
      snapshot: {} as never,
    };

    render(<DashboardScreen />);

    expect(screen.getByRole("main").className).toBe(
      EXPECTED_DASHBOARD_SHELL_CLASS,
    );
    expect(screen.getByText("Dashboard header session-review en")).toBeTruthy();
    expect(screen.getByText("Dashboard bento en")).toBeTruthy();
    expect(screen.getByText("Dashboard export dialog en")).toBeTruthy();
  });
});
