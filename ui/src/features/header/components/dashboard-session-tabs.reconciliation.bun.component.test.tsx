import { afterEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";

import { FACTORY_SESSIONS_QUERY_KEY } from "../../../api/factory-sessions/query-keys";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  createDashboardRegressionFixture,
  DASHBOARD_REGRESSION_SESSION_IDS,
} from "../../../components/dashboard/fixtures";
import { bunVi as vi } from "../../../testing/bun/vi-compat";
import { resetDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardSessionTabs } from "./dashboard-session-tabs";

describe("DashboardSessionTabs canonical reconciliation", () => {
  afterEach(() => {
    resetDashboardSessionStore();
    vi.unstubAllGlobals();
  });

  it("renders canonical rows once and reconciles refreshed membership", async () => {
    const fixture = createDashboardRegressionFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const queryClient = createQueryClient();

    const rendered = renderWithQueryClient(queryClient);
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });

    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });
    await waitFor(() => {
      expect(screen.getAllByRole("tab")).toHaveLength(2);
    });

    expect(
      screen.getAllByRole("tab").map((tab) => tab.textContent),
    ).not.toContain(DEFAULT_FACTORY_SESSION_ID);
    expect(fixture.state().currentSessionIDs).toEqual([
      DASHBOARD_REGRESSION_SESSION_IDS.default,
      DASHBOARD_REGRESSION_SESSION_IDS.secondary,
    ]);

    fixture.sessionLists.enqueueFetch("refreshed");
    let refreshedRefresh!: Promise<void>;
    await act(async () => {
      refreshedRefresh = queryClient.refetchQueries({
        queryKey: FACTORY_SESSIONS_QUERY_KEY,
      });
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("refreshed");
      expect(screen.getByRole("status").textContent).toBe(
        "Refreshing sessions...",
      );
    });

    await act(async () => {
      fixture.sessionLists.resolve("refreshed");
      await refreshedRefresh;
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "created" })).toBeTruthy();
      expect(screen.queryByRole("tab", { name: "secondary" })).toBeNull();
      expect(screen.queryByRole("tab", { name: "removed" })).toBeNull();
    });

    expect(fixture.state().currentSessionListID).toBe("refreshed");
    expect(screen.queryByText(DEFAULT_FACTORY_SESSION_ID)).toBeNull();
    await act(async () => {
      rendered.unmount();
      queryClient.clear();
    });
  });

  it("keeps authoritative rows visible but labels a failed refresh and exposes retry", async () => {
    const fixture = createDashboardRegressionFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const queryClient = createQueryClient();

    const rendered = renderWithQueryClient(queryClient);
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });
    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "secondary" })).toBeTruthy();
    });

    fixture.sessionLists.enqueueFetch("stale");
    let failedRefresh!: Promise<void>;
    await act(async () => {
      failedRefresh = queryClient.refetchQueries({
        queryKey: FACTORY_SESSIONS_QUERY_KEY,
      });
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("stale");
    });
    await act(async () => {
      fixture.sessionLists.reject("stale", new Error("refresh failed"));
      await failedRefresh;
    });
    const messages = getHeaderControlsMessages("en");
    expect(await screen.findByText(messages.sessionsOfflineTitle)).toBeTruthy();
    expect(
      screen.getByRole("button", { name: messages.retrySessionsLabel }),
    ).toBeTruthy();
    expect(screen.getByRole("tab", { name: "secondary" })).toBeTruthy();
    await act(async () => {
      rendered.unmount();
      queryClient.clear();
    });
  });
});

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function renderWithQueryClient(queryClient: QueryClient) {
  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTabs locale="en" />
    </QueryClientProvider>,
  );
}
