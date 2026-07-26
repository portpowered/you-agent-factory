import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { resetDashboardSessionStore } from "../state/dashboardSessionStore";
import { DashboardScreen } from "./dashboard-screen";

const useDashboardSnapshot = vi.fn(() => ({
  error: null,
  isInitialLoading: true,
  preflightRecovery: null,
  preflightStatus: "loading" as const,
  snapshot: null,
  streamState: {
    message: "Loading factory events...",
    status: "connecting" as const,
  },
}));

vi.mock("../../bento/components/dashboard-bento", () => ({
  DashboardBento: () => <section>Dashboard bento</section>,
}));

vi.mock("../../header/components/dashboard-export-dialog", () => ({
  DashboardExportDialog: () => null,
}));

vi.mock("../../header/components/dashboard-header", () => ({
  DashboardHeader: () => <header>Dashboard header</header>,
}));

vi.mock("../../header/components/dashboard-status-panel", () => ({
  DashboardStatusPanel: ({
    actions,
    detail,
    title,
  }: {
    actions?: ReactNode;
    detail?: string;
    title: string;
  }) => (
    <section>
      <h1>{title}</h1>
      {detail ? <p>{detail}</p> : null}
      {actions}
    </section>
  ),
}));

vi.mock("../hooks/useDashboardSnapshot", () => ({
  useDashboardSnapshot: () => useDashboardSnapshot(),
}));

function sessionListResponse(sessions: unknown[]) {
  return new Response(JSON.stringify({ sessions }), {
    headers: { "Content-Type": "application/json" },
    status: 200,
  });
}

function resolvedDefaultSession() {
  return {
    factoryDir: "/workspace/root",
    folderPath: "/workspace/root",
    id: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
    isDefault: true,
    project: "root",
    target: { kind: "default" },
  };
}

function renderScreen() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardScreen />
    </QueryClientProvider>,
  );
}

describe("DashboardScreen default session discovery", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
    useDashboardSnapshot.mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows discovery loading without mounting session-scoped hooks", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    const messages = getHeaderControlsMessages("en");

    renderScreen();

    expect(
      screen.getByRole("heading", { name: messages.loadingSessionsLabel }),
    ).toBeTruthy();
    expect(useDashboardSnapshot).not.toHaveBeenCalled();
  });

  it("shows the empty discovery state without guessing an identity", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => sessionListResponse([])),
    );
    const messages = getHeaderControlsMessages("en");

    renderScreen();

    expect(
      await screen.findByRole("heading", { name: messages.sessionsEmptyTitle }),
    ).toBeTruthy();
    expect(screen.getByRole("banner").textContent).toBe("Dashboard header");
    expect(useDashboardSnapshot).not.toHaveBeenCalled();
  });

  it("recovers from discovery failure through the retry action", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockRejectedValueOnce(new Error("network down"))
        .mockResolvedValueOnce(sessionListResponse([resolvedDefaultSession()])),
    );
    const messages = getHeaderControlsMessages("en");

    renderScreen();

    const retry = await screen.findByRole("button", {
      name: messages.retrySessionsLabel,
    });
    expect(
      screen.getByText(
        "The dashboard could not reach the factory sessions API.",
      ),
    ).toBeTruthy();
    expect(useDashboardSnapshot).not.toHaveBeenCalled();

    fireEvent.click(retry);

    expect(
      await screen.findByRole("heading", {
        name: messages.loadingDashboardTitle,
      }),
    ).toBeTruthy();
    expect(useDashboardSnapshot).toHaveBeenCalled();
  });
});
