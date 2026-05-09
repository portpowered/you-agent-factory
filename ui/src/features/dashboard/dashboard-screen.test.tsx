import { render, screen } from "@testing-library/react";

import { DashboardScreen } from "./dashboard-screen";

let dashboardSnapshotState: ReturnType<
  typeof import("./useDashboardSnapshot").useDashboardSnapshot
>;

vi.mock("../bento", () => ({
  DashboardBento: () => <section>Dashboard bento</section>,
}));

vi.mock("../header", () => ({
  DashboardExportDialog: () => <div>Dashboard export dialog</div>,
  DashboardHeader: () => <header>Dashboard header</header>,
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

vi.mock("./useDashboardSnapshot", () => ({
  useDashboardSnapshot: vi.fn(() => dashboardSnapshotState),
}));

describe("DashboardScreen", () => {
  beforeEach(() => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: true,
      snapshot: null,
    };
  });

  it("uses the tighter dashboard shell spacing while loading", () => {
    render(<DashboardScreen />);

    expect(screen.getByRole("main").className).toContain("p-2");
    expect(screen.getByRole("main").className).not.toContain("p-5");
    expect(
      screen.getByRole("heading", { name: "Loading dashboard" }),
    ).toBeTruthy();
  });

  it("keeps the tighter dashboard shell spacing when the dashboard request fails", () => {
    dashboardSnapshotState = {
      error: new Error("Factory API timed out."),
      isInitialLoading: false,
      snapshot: null,
    };

    render(<DashboardScreen />);

    expect(screen.getByRole("main").className).toContain("p-2");
    expect(
      screen.getByRole("heading", { name: "Dashboard unavailable" }),
    ).toBeTruthy();
    expect(screen.getByText("Factory API timed out.")).toBeTruthy();
  });

  it("renders the dashboard content inside the tighter shell spacing on success", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      snapshot: {} as never,
    };

    render(<DashboardScreen />);

    expect(screen.getByRole("main").className).toContain("p-2");
    expect(screen.getByText("Dashboard header")).toBeTruthy();
    expect(screen.getByText("Dashboard bento")).toBeTruthy();
    expect(screen.getByText("Dashboard export dialog")).toBeTruthy();
  });
});
