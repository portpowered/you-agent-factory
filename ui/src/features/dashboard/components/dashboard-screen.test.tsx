// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: dashboard shell loading, error, and success paths share one snapshot harness.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";

import { AppLocaleProvider, useAppLocale } from "../../../i18n";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { getDashboardRecoveryMessages } from "../messages/dashboard-recovery";
import { DashboardScreen } from "./dashboard-screen";

const EXPECTED_DASHBOARD_SHELL_CLASS =
  "min-h-screen overflow-x-hidden p-1 md:p-2";
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
  actions,
  detail,
  title,
}: {
  actions?: ReactNode;
  detail?: string;
  title: string;
}) {
  const { locale } = useAppLocale();

  return (
    <section data-locale={locale}>
      <h1>{title}</h1>
      {detail ? <p>{detail}</p> : null}
      {actions}
    </section>
  );
}

vi.mock("../../bento/public", () => ({
  DashboardBento: ({
    locale,
    workOutcomeStream,
  }: {
    locale?: string;
    workOutcomeStream?: {
      identity: { streamGenerationID: string } | null;
      status: "loading" | "ready";
    };
  }) => {
    const { locale: resolvedLocale } = useAppLocale(locale);
    return (
      <section
        data-stream-generation={workOutcomeStream?.identity?.streamGenerationID}
        data-stream-status={workOutcomeStream?.status}
        data-testid="dashboard-bento-probe"
      >
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
    actions,
    detail,
    title,
  }: {
    actions?: ReactNode;
    detail?: string;
    title: string;
  }) => <StatusPanelProbe actions={actions} detail={detail} title={title} />,
}));

vi.mock("../hooks/useDashboardSnapshot", () => ({
  useDashboardSnapshot: vi.fn(
    ({ refreshToken = 0 }: { refreshToken?: number } = {}) =>
      dashboardSnapshotResolver?.(refreshToken) ?? dashboardSnapshotState,
  ),
}));

vi.mock("../session/dashboard-session-provider", () => ({
  DashboardSessionProvider: ({ children }: { children: ReactNode }) => children,
  useDashboardSession: () => ({ rawSessionID: "session-test" }),
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

describe("DashboardScreen loading and error states", () => {
  beforeEach(() => {
    dashboardSnapshotResolver = null;
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
      preflightRecovery: null,
      preflightStatus: "success",
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
      preflightRecovery: null,
      preflightStatus: "success",
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
});

describe("DashboardScreen content states", () => {
  beforeEach(() => {
    dashboardSnapshotResolver = null;
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

  it("does not render the session lifecycle banner for an active session", () => {
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

    render(<DashboardScreen />);

    expect(
      screen.queryByTestId("dashboard-session-lifecycle-banner"),
    ).toBeNull();
    expect(screen.queryByText("Session started")).toBeNull();
    expect(screen.getByText("Dashboard header en")).toBeTruthy();
    expect(screen.getByText("Dashboard bento en")).toBeTruthy();
  });

  it("renders the dashboard content inside the tighter shell spacing on success", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      preflightRecovery: null,
      preflightStatus: "success",
      snapshot: {} as never,
      streamState: {
        message: "Factory event stream connected.",
        status: "live",
      },
      workOutcomeStreamIdentity: {
        backendScopeID: "backend-a",
        factorySessionID: "session-a",
        logicalSessionKeyID: "logical-a",
        streamGenerationID: "generation-a",
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
    expect(screen.getByTestId("dashboard-bento-probe").dataset).toMatchObject({
      streamGeneration: "generation-a",
      streamStatus: "ready",
    });
  });

  it("keeps the work outcome card in hydration mode while preflight is pending", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      preflightRecovery: null,
      preflightStatus: "loading",
      snapshot: {} as never,
      streamState: {
        message: "Validating retained history.",
        status: "connecting",
      },
    };

    render(<DashboardScreen />);

    expect(screen.getByTestId("dashboard-bento-probe").dataset).toMatchObject({
      streamStatus: "loading",
    });
  });

  it("does not introduce a nested vertical scroll owner on the success path", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      preflightRecovery: null,
      preflightStatus: "success",
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
      preflightRecovery: null,
      preflightStatus: "success",
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
      preflightRecovery: null,
      preflightStatus: "success",
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
});

describe("DashboardScreen recovery states", () => {
  beforeEach(() => {
    dashboardSnapshotResolver = null;
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
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
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
            preflightRecovery: null,
            preflightStatus: "success",
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
            preflightRecovery: null,
            preflightStatus: "loading",
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

  it("renders a recoverable preflight reset state instead of the generic offline error", () => {
    dashboardSnapshotState = {
      error: null,
      isInitialLoading: false,
      preflightRecovery: {
        reasonCode: "session_not_found",
        requestedSessionId: "session-review",
      },
      preflightStatus: "non-recoverable",
      snapshot: null,
      streamState: {
        message: "Factory event stream disconnected. Showing last event state.",
        status: "offline",
      },
    };

    render(<DashboardScreen />);

    expectDashboardShellContract();
    expect(screen.getByText("Dashboard header en")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Session recovery required" }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        /could not resolve the live session for "session-review"/i,
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Retry clean replay" }),
    ).toBeTruthy();
    expect(screen.queryByText("Dashboard bento en")).toBeNull();
  });
});
