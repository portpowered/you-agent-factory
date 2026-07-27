import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { buildSessionScope } from "../../../api/session-scope";
import { bunVi as vi } from "../../../testing/bun/vi-compat";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../state/dashboardSessionStore";
import {
  type DashboardSessionDiscoveryState,
  DashboardSessionProvider,
  DashboardSessionScopeProvider,
  useDashboardSession,
} from "./dashboard-session-provider";

const RESOLVED_DEFAULT_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function sessionListResponse() {
  return new Response(
    JSON.stringify({
      sessions: [
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: RESOLVED_DEFAULT_SESSION_UUID,
          isDefault: true,
          project: "root",
          target: { kind: "default" },
        },
        {
          factoryDir: "/workspace/beta",
          folderPath: "/workspace/beta",
          id: "session-beta",
          isDefault: false,
          project: "beta",
          target: { kind: "named", name: "beta" },
        },
      ],
    }),
    { headers: { "Content-Type": "application/json" }, status: 200 },
  );
}

function DiscoveryStateProbe({
  state,
}: {
  state: DashboardSessionDiscoveryState;
}) {
  if (state.status === "error") {
    return (
      <button onClick={state.retry} type="button">
        error: retry
      </button>
    );
  }
  return <div>{state.status}</div>;
}

function renderProvider(children: ReactNode = <SessionScopeProbe />) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionProvider
        renderDiscoveryState={(state) => <DiscoveryStateProbe state={state} />}
      >
        {children}
      </DashboardSessionProvider>
    </QueryClientProvider>,
  );
}

function SessionScopeProbe() {
  const { eventsPath, factoryPath, isDefault, isPaused, sessionID, workPath } =
    useDashboardSession();

  return (
    <div data-testid="session-scope-probe">
      {sessionID}|{factoryPath}|{workPath}|{eventsPath}|{String(isPaused)}|
      {String(isDefault)}
    </div>
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: provider discovery, retry, and scope outcomes share one lifecycle harness.
describe("DashboardSessionProvider", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => sessionListResponse()),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps the default selector transient until discovery resolves its UUID", async () => {
    let resolveDiscovery: ((response: Response) => void) | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveDiscovery = resolve;
          }),
      ),
    );
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });

    renderProvider();

    expect(screen.getByText("loading")).toBeTruthy();
    expect(screen.queryByTestId("session-scope-probe")).toBeNull();
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );

    await act(async () => {
      resolveDiscovery?.(sessionListResponse());
    });

    expect((await screen.findByTestId("session-scope-probe")).textContent).toBe(
      `${RESOLVED_DEFAULT_SESSION_UUID}|/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/factory|/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/work|/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events|false|true`,
    );
    await waitFor(() => {
      expect(useDashboardSessionStore.getState()).toMatchObject({
        selectedSessionID: RESOLVED_DEFAULT_SESSION_UUID,
        sessionTabOrder: [RESOLVED_DEFAULT_SESSION_UUID, "session-beta"],
      });
    });
  });

  it("updates scope when setSelectedSessionID changes", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: "~default",
    });

    renderProvider();

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    expect(screen.getByTestId("session-scope-probe").textContent).toBe(
      "session-beta|/factory-sessions/session-beta/factory|/factory-sessions/session-beta/work|/factory-sessions/session-beta/events|false|false",
    );
  });

  it("reflects pause state for the selected session", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: ["session-beta"],
      selectedSessionID: "session-beta",
    });

    renderProvider();

    expect(screen.getByTestId("session-scope-probe").textContent).toContain(
      "|true",
    );
  });

  it("throws when useDashboardSession is called outside the provider", () => {
    expect(() => render(<SessionScopeProbe />)).toThrow(
      "useDashboardSession must be used within DashboardSessionProvider.",
    );
  });

  it("preserves an inherited test scope without running session discovery", () => {
    const fetchMock = vi.mocked(fetch);

    render(
      <DashboardSessionScopeProvider scope={buildSessionScope(null, [], false)}>
        <DashboardSessionProvider>
          <SessionScopeProbe />
        </DashboardSessionProvider>
      </DashboardSessionScopeProvider>,
    );

    expect(screen.getByTestId("session-scope-probe").textContent).toBe(
      "~default|/factory-sessions/~default/factory|/factory-sessions/~default/work|/factory-sessions/~default/events|false|true",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("does not guess a concrete identity when discovery is empty", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ sessions: [] }), {
            headers: { "Content-Type": "application/json" },
            status: 200,
          }),
      ),
    );

    renderProvider();

    expect(await screen.findByText("empty")).toBeTruthy();
    expect(screen.queryByTestId("session-scope-probe")).toBeNull();
    expect(useDashboardSessionStore.getState()).toMatchObject({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
      sessionTabOrder: [],
    });
  });

  it("keeps discovery failure explicit and resolves on retry", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockRejectedValueOnce(new Error("network down"))
        .mockResolvedValueOnce(sessionListResponse()),
    );

    renderProvider();

    const retry = await screen.findByRole("button", { name: "error: retry" });
    expect(screen.queryByTestId("session-scope-probe")).toBeNull();
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );

    fireEvent.click(retry);

    expect(await screen.findByTestId("session-scope-probe")).toBeTruthy();
    await waitFor(() => {
      expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
        RESOLVED_DEFAULT_SESSION_UUID,
      );
    });
  });

  it("keeps the resolved default and named session distinct", async () => {
    renderProvider();

    await screen.findByTestId("session-scope-probe");
    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    expect(screen.getByTestId("session-scope-probe").textContent).toBe(
      "session-beta|/factory-sessions/session-beta/factory|/factory-sessions/session-beta/work|/factory-sessions/session-beta/events|false|false",
    );
    expect(useDashboardSessionStore.getState().sessionTabOrder).toEqual([
      RESOLVED_DEFAULT_SESSION_UUID,
      "session-beta",
    ]);
  });
});
