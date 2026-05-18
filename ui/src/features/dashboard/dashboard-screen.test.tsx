import { render, screen } from "@testing-library/react";

import { getHeaderControlsMessages } from "../header/messages/header-controls";
import { DashboardScreen } from "./dashboard-screen";

const EXPECTED_DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";

let dashboardSnapshotState: ReturnType<
  typeof import("./useDashboardSnapshot").useDashboardSnapshot
>;

vi.mock("../bento", () => ({
  DashboardBento: () => <section>Dashboard bento</section>,
}));

vi.mock("../header", () => ({
  DashboardExportDialog: ({ locale }: { locale?: string }) => (
    <div>Dashboard export dialog {locale}</div>
  ),
  DashboardHeader: ({ locale }: { locale?: string }) => (
    <header>Dashboard header {locale}</header>
  ),
  DashboardStatusPanel: ({
    detail,
    locale,
    title,
  }: {
    detail?: string;
    locale?: string;
    title: string;
  }) => (
    <section data-locale={locale}>
      <h1>{title}</h1>
      {detail ? <p>{detail}</p> : null}
    </section>
  ),
}));

vi.mock("./useDashboardSnapshot", () => ({
  useDashboardSnapshot: vi.fn(() => dashboardSnapshotState),
}));

describe("DashboardScreen", () => {
  function expectDashboardShellContract() {
    expect(screen.getByRole("main").className).toBe(
      EXPECTED_DASHBOARD_SHELL_CLASS,
    );
  }

  beforeEach(() => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: true,
      snapshot: null,
    };
  });

  it("uses the tighter dashboard shell spacing while loading", () => {
    render(<DashboardScreen />);
    const messages = getHeaderControlsMessages("en");

    expectDashboardShellContract();
    expect(
      screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
    ).toBeTruthy();
  });

  it("keeps the tighter dashboard shell spacing when the dashboard request fails", () => {
    const messages = getHeaderControlsMessages("en");

    dashboardSnapshotState = {
      error: new Error("Factory API timed out."),
      isInitialLoading: false,
      snapshot: null,
    };

    render(<DashboardScreen />);

    expectDashboardShellContract();
    expect(
      screen.getByRole("heading", { name: messages.dashboardUnavailableTitle }),
    ).toBeTruthy();
    expect(screen.getByText("Factory API timed out.")).toBeTruthy();
  });

  it("renders localized loading and error shell titles", () => {
    const messages = getHeaderControlsMessages("zh-CN");
    const { rerender } = render(<DashboardScreen locale="zh-CN" />);

    expect(
      screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
    ).toBeTruthy();
    expect(screen.getByRole("heading").closest("section")?.dataset.locale).toBe(
      "zh-CN",
    );

    dashboardSnapshotState = {
      error: new Error("Factory API timed out."),
      isInitialLoading: false,
      snapshot: null,
    };
    rerender(<DashboardScreen locale="zh-CN" />);

    expect(
      screen.getByRole("heading", {
        name: messages.dashboardUnavailableTitle,
      }),
    ).toBeTruthy();
  });

  it("renders the dashboard content inside the tighter shell spacing on success", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      snapshot: {} as never,
    };

    render(<DashboardScreen locale="zh-CN" />);

    expectDashboardShellContract();
    expect(screen.getByText("Dashboard header zh-CN")).toBeTruthy();
    expect(screen.getByText("Dashboard bento")).toBeTruthy();
    expect(screen.getByText("Dashboard export dialog zh-CN")).toBeTruthy();
  });
});
