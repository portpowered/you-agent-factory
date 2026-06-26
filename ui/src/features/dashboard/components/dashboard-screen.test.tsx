import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { AppLocaleProvider, useAppLocale } from "../../../i18n";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { getDashboardRecoveryMessages } from "../messages/dashboard-recovery";
import { DashboardScreen } from "./dashboard-screen";

const EXPECTED_DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";
const VERTICAL_SCROLL_CLASS_PATTERN =
  /(?:^|\s)(?:overflow-(?:auto|scroll)|overflow-y-(?:auto|scroll))(?:\s|$)/;
const VIEWPORT_HEIGHT_CLAMP_CLASS_PATTERN =
  /(?:^|\s)(?:h-(?:screen|dvh|svh|lvh)|max-h-(?:screen|dvh|svh|lvh))(?:\s|$)/;

let dashboardSnapshotState: ReturnType<
  typeof import("../hooks/useDashboardSnapshot").useDashboardSnapshot
>;
let dashboardSnapshotResolver:
  | ((refreshToken: number) => typeof dashboardSnapshotState)
  | null;

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
  useDashboardSnapshot: vi.fn(
    ({ refreshToken = 0 }: { refreshToken?: number } = {}) =>
      dashboardSnapshotResolver?.(refreshToken) ?? dashboardSnapshotState,
  ),
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: dashboard shell coverage keeps success and failure states together.
describe("DashboardScreen", () => {
  beforeEach(() => {
    dashboardSnapshotResolver = null;
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: true,
      snapshot: null,
      streamState: {
        message: "Loading factory events...",
        status: "connecting",
      },
    };
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
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
      streamState: {
        message: "Factory event stream disconnected. Showing last event state.",
        status: "offline",
      },
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
      streamState: {
        message: "Factory event stream disconnected. Showing last event state.",
        status: "offline",
      },
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
      streamState: {
        message: "Factory event stream connected.",
        status: "live",
      },
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
      streamState: {
        message: "Factory event stream connected.",
        status: "live",
      },
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
      streamState: {
        message: "Factory event stream connected.",
        status: "live",
      },
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
      streamState: {
        message: "Factory event stream connected.",
        status: "live",
      },
    };

    render(<DashboardScreen locale="zh-CN" />);

    expect(screen.getByText("Dashboard header zh-CN")).toBeTruthy();
    expect(screen.getByText("Dashboard bento zh-CN")).toBeTruthy();
    expect(screen.getByText("Dashboard export dialog zh-CN")).toBeTruthy();
  });

  it("shows a recoverable replay failure state and retries the session stream", async () => {
    const user = userEvent.setup();
    const messages = getHeaderControlsMessages("en");
    const recoveryMessages = getDashboardRecoveryMessages("en");
    dashboardSnapshotResolver = (refreshToken) =>
      refreshToken === 0
        ? {
            error: new Error(
              "The dashboard could not restore this session automatically.",
            ),
            isInitialLoading: false,
            snapshot: null,
            streamState: {
              message:
                "The dashboard could not restore this session automatically.",
              status: "recovery_failed",
            },
          }
        : {
            error: null,
            isInitialLoading: true,
            snapshot: null,
            streamState: {
              message: "Loading factory events...",
              status: "connecting",
            },
          };

    render(<DashboardScreen />);

    expect(
      screen.getByRole("heading", {
        name: recoveryMessages.recoveryFailedTitle,
      }),
    ).toBeTruthy();
    expect(
      screen.getByText(recoveryMessages.recoveryFailedDetail),
    ).toBeTruthy();

    await user.click(
      screen.getByRole("button", {
        name: recoveryMessages.recoveryFailedRetryLabel,
      }),
    );

    expect(
      screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
    ).toBeTruthy();
  });
});
