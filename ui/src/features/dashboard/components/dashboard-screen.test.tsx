import { render, screen } from "@testing-library/react";

import { AppLocaleProvider, useAppLocale } from "../../../i18n";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { DashboardScreen } from "./dashboard-screen";

const EXPECTED_DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";
const VERTICAL_SCROLL_CLASS_PATTERN =
  /(?:^|\s)(?:overflow-(?:auto|scroll)|overflow-y-(?:auto|scroll))(?:\s|$)/;
const VIEWPORT_HEIGHT_CLAMP_CLASS_PATTERN =
  /(?:^|\s)(?:h-(?:screen|dvh|svh|lvh)|max-h-(?:screen|dvh|svh|lvh))(?:\s|$)/;

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
    return (
      <section data-testid="dashboard-bento-probe">
        Dashboard bento {resolvedLocale}
      </section>
    );
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

function expectDashboardShellContract() {
  const shell = screen.getByRole("main");

  expect(shell.className).toBe(EXPECTED_DASHBOARD_SHELL_CLASS);
  expectElementDoesNotOwnVerticalScroll(shell);
  expect(shell.getAttribute("class")).not.toMatch(
    VIEWPORT_HEIGHT_CLAMP_CLASS_PATTERN,
  );
}

function expectElementDoesNotOwnVerticalScroll(element: HTMLElement) {
  expect(element.getAttribute("class") ?? "").not.toMatch(
    VERTICAL_SCROLL_CLASS_PATTERN,
  );
  expect(window.getComputedStyle(element).overflowY).not.toMatch(
    /^(auto|scroll)$/,
  );
}

function expectNoNestedDashboardScrollOwnerBetweenBentoAndShell() {
  const shell = screen.getByRole("main");
  const bento = screen.getByTestId("dashboard-bento-probe");
  let currentElement: HTMLElement | null = bento;

  while (currentElement && currentElement !== shell) {
    expectElementDoesNotOwnVerticalScroll(currentElement);
    currentElement = currentElement.parentElement;
  }

  expect(currentElement).toBe(shell);
  expectDashboardShellContract();
}

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

  it("does not introduce a nested vertical scroll owner on the success path", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      snapshot: {} as never,
    };

    render(<DashboardScreen />);

    expectNoNestedDashboardScrollOwnerBetweenBentoAndShell();
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
