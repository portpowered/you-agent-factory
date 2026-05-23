import { render, screen } from "@testing-library/react";

import { AppLocaleProvider, useAppLocale } from "../../../i18n";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { DashboardScreen } from "./dashboard-screen";

const EXPECTED_DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";

let dashboardSnapshotState: ReturnType<
  typeof import("../hooks/useDashboardSnapshot").useDashboardSnapshot
>;

function StatusPanelProbe({
  detail,
  title,
}: {
  detail?: string;
  title: string;
}) {
  const { locale } = useAppLocale();

  return (
    <section data-locale={locale}>
      <h1>{title}</h1>
      {detail ? <p>{detail}</p> : null}
    </section>
  );
}

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
    return <header>Dashboard header {resolvedLocale}</header>;
  },
  DashboardStatusPanel: ({
    detail,
    title,
  }: {
    detail?: string;
    title: string;
  }) => <StatusPanelProbe detail={detail} title={title} />,
}));

vi.mock("../hooks/useDashboardSnapshot", () => ({
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
    const { rerender } = render(
      <AppLocaleProvider initialLocale="zh-CN">
        <DashboardScreen />
      </AppLocaleProvider>,
    );

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
    rerender(
      <AppLocaleProvider initialLocale="zh-CN">
        <DashboardScreen />
      </AppLocaleProvider>,
    );

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

    render(
      <AppLocaleProvider initialLocale="zh-CN">
        <DashboardScreen />
      </AppLocaleProvider>,
    );

    expectDashboardShellContract();
    expect(screen.getByText("Dashboard header zh-CN")).toBeTruthy();
    expect(screen.getByText("Dashboard bento zh-CN")).toBeTruthy();
    expect(screen.getByText("Dashboard export dialog zh-CN")).toBeTruthy();
  });

  it("keeps the header visible and renders an empty workspace state when no live session remains", () => {
    const messages = getHeaderControlsMessages("en");
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      snapshot: null,
    };

    render(<DashboardScreen />);

    expectDashboardShellContract();
    expect(screen.getByText("Dashboard header en")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: messages.sessionsEmptyTitle }),
    ).toBeTruthy();
    expect(screen.queryByText("Dashboard bento en")).toBeNull();
    expect(screen.queryByText("Dashboard export dialog en")).toBeNull();
  });

  it("keeps direct locale overrides available to the dashboard children", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      snapshot: {} as never,
    };

    render(<DashboardScreen locale="zh-CN" />);

    expect(screen.getByText("Dashboard header zh-CN")).toBeTruthy();
    expect(screen.getByText("Dashboard bento zh-CN")).toBeTruthy();
    expect(screen.getByText("Dashboard export dialog zh-CN")).toBeTruthy();
  });
});
