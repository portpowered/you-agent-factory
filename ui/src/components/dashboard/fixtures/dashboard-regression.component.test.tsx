import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FACTORY_SESSIONS_QUERY_KEY } from "../../../api/factory-sessions/query-keys";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  DashboardSessionProvider,
  useDashboardSession,
} from "../../../features/dashboard/session/dashboard-session-provider";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../../../features/dashboard/state/dashboardSessionStore";
import {
  createDashboardRegressionFixture,
  DASHBOARD_REGRESSION_SESSION_IDS,
} from ".";

function ScopeProbe() {
  const { sessionID, factoryPath, workPath, eventsPath } =
    useDashboardSession();
  return (
    <output data-testid="dashboard-regression-scope">
      {sessionID}|{factoryPath}|{workPath}|{eventsPath}
    </output>
  );
}

function renderProvider(
  queryClient: QueryClient,
  children: ReactNode = <ScopeProbe />,
) {
  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionProvider
        renderDiscoveryState={(state) => (
          <div data-testid="dashboard-regression-discovery">{state.status}</div>
        )}
      >
        {children}
      </DashboardSessionProvider>
    </QueryClientProvider>,
  );
}

describe("dashboard regression fixture typed session boundary", () => {
  afterEach(() => {
    resetDashboardSessionStore();
    vi.unstubAllGlobals();
  });

  it("resolves ~default once and seeds the query/store with canonical session IDs", async () => {
    const fixture = createDashboardRegressionFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    renderProvider(queryClient);
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });

    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });

    expect(
      (await screen.findByTestId("dashboard-regression-scope")).textContent,
    ).toBe(
      `${DASHBOARD_REGRESSION_SESSION_IDS.default}|/factory-sessions/${DASHBOARD_REGRESSION_SESSION_IDS.default}/factory|/factory-sessions/${DASHBOARD_REGRESSION_SESSION_IDS.default}/work|/factory-sessions/${DASHBOARD_REGRESSION_SESSION_IDS.default}/events`,
    );
    await waitFor(() => {
      expect(useDashboardSessionStore.getState()).toMatchObject({
        selectedSessionID: DASHBOARD_REGRESSION_SESSION_IDS.default,
        sessionTabOrder: [
          DASHBOARD_REGRESSION_SESSION_IDS.default,
          DASHBOARD_REGRESSION_SESSION_IDS.secondary,
        ],
      });
    });

    const sessions = queryClient.getQueryData<ReadonlyArray<{ id: string }>>(
      FACTORY_SESSIONS_QUERY_KEY,
    );
    expect(sessions?.map((session) => session.id)).toEqual([
      DASHBOARD_REGRESSION_SESSION_IDS.default,
      DASHBOARD_REGRESSION_SESSION_IDS.secondary,
    ]);
    expect(sessions?.map((session) => session.id)).not.toContain(
      DEFAULT_FACTORY_SESSION_ID,
    );
  });
});
