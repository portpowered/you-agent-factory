import { afterEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";

import { FACTORY_SESSIONS_QUERY_KEY } from "../../../api/factory-sessions/query-keys";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  createDashboardRegressionFixture,
  DASHBOARD_REGRESSION_SESSION_IDS,
} from "../../../components/dashboard/fixtures";
import { bunVi as vi } from "../../../testing/bun/vi-compat";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../../dashboard/state/dashboardSessionStore";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardSessionTabs } from "./dashboard-session-tabs";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: keeps the fixture scenarios in one observable reconciliation suite.
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

  it("keeps selection and focus aligned when reconciliation removes the active tab", async () => {
    const fixture = createDashboardRegressionFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const queryClient = createQueryClient();

    renderWithQueryClient(queryClient);
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });
    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "secondary" })).toBeTruthy();
    });

    const secondaryTab = screen.getByRole("tab", { name: "secondary" });
    fireEvent.click(secondaryTab);
    await waitFor(() => {
      expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
        DASHBOARD_REGRESSION_SESSION_IDS.secondary,
      );
      expect(document.activeElement).toBe(secondaryTab);
    });

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
    });
    await act(async () => {
      fixture.sessionLists.resolve("refreshed");
      await refreshedRefresh;
    });

    const defaultTab = await screen.findByRole("tab", {
      name: "dashboard-regression",
    });
    const createdTab = screen.getByRole("tab", { name: "created" });
    await waitFor(() => {
      expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
        DASHBOARD_REGRESSION_SESSION_IDS.default,
      );
      expect(defaultTab.getAttribute("aria-selected")).toBe("true");
      expect(defaultTab.getAttribute("tabindex")).toBe("0");
      expect(createdTab.getAttribute("aria-selected")).toBe("false");
      expect(createdTab.getAttribute("tabindex")).toBe("-1");
      expect(document.activeElement).toBe(defaultTab);
    });

    fireEvent.keyDown(defaultTab, { key: "End" });
    await waitFor(() => {
      expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
        DASHBOARD_REGRESSION_SESSION_IDS.created,
      );
      expect(createdTab.getAttribute("aria-selected")).toBe("true");
      expect(createdTab.getAttribute("tabindex")).toBe("0");
      expect(defaultTab.getAttribute("aria-selected")).toBe("false");
      expect(defaultTab.getAttribute("tabindex")).toBe("-1");
      expect(document.activeElement).toBe(createdTab);
    });

    fireEvent.keyDown(createdTab, { key: "Home" });
    await waitFor(() => {
      expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
        DASHBOARD_REGRESSION_SESSION_IDS.default,
      );
      expect(defaultTab.getAttribute("aria-selected")).toBe("true");
      expect(defaultTab.getAttribute("tabindex")).toBe("0");
      expect(createdTab.getAttribute("aria-selected")).toBe("false");
      expect(createdTab.getAttribute("tabindex")).toBe("-1");
      expect(document.activeElement).toBe(defaultTab);
    });
  });

  it("keeps Open Factory selection non-mutating until explicit confirmation", async () => {
    const fixture = createDashboardRegressionFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const queryClient = createQueryClient();

    renderWithQueryClient(queryClient);
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });
    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "secondary" })).toBeTruthy();
    });

    const messages = getHeaderControlsMessages("en");
    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      { target: { value: "/workspace/dashboard-regression" } },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );
    await waitFor(() => {
      expect(fixture.state().pendingFactoryOperationIDs).toContain(
        "open-validation-success",
      );
    });
    await act(async () => {
      fixture.factoryJourneys.resolve("open-validation-success");
    });

    const targetPicker = await screen.findByRole("region", {
      name: messages.targetPickerTitle,
    });
    const targetButton = within(targetPicker).getByRole("button", {
      name: /secondary/,
    });
    fireEvent.click(targetButton);
    expect(fixture.state().pendingFactoryOperationIDs).not.toContain(
      "open-confirm-success",
    );

    fixture.sessionLists.enqueueFetch("initial");
    fireEvent.click(
      within(targetPicker).getByRole("button", {
        name: messages.openSessionTargetLabel,
      }),
    );
    await waitFor(() => {
      expect(fixture.state().pendingFactoryOperationIDs).toContain(
        "open-confirm-success",
      );
    });
    await act(async () => {
      fixture.factoryJourneys.resolve("open-confirm-success");
    });
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });
    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });
    await waitFor(() => {
      expect(
        screen
          .getByRole("tab", { name: "secondary" })
          .getAttribute("aria-selected"),
      ).toBe("true");
      expect(screen.getByRole("status").textContent).toBe(
        messages.openFactorySuccessLabel,
      );
    });
  });

  it("keeps New Factory validation and cancel non-mutating before preview confirmation", async () => {
    const fixture = createDashboardRegressionFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const queryClient = createQueryClient();

    renderWithQueryClient(queryClient);
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });
    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "secondary" })).toBeTruthy();
    });

    const messages = getHeaderControlsMessages("en");
    fireEvent.click(
      screen.getByRole("button", { name: messages.newFactoryButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.newFactoryFolderFieldPlaceholder),
      { target: { value: "/workspace/dashboard-regression-new" } },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );
    await waitFor(() => {
      expect(fixture.state().pendingFactoryOperationIDs).toContain(
        "new-validation-success",
      );
    });
    await act(async () => {
      fixture.factoryJourneys.resolve("new-validation-success");
    });
    await waitFor(() => {
      expect(
        screen.getByText("/workspace/dashboard-regression-new/factory"),
      ).toBeTruthy();
    });
    expect(fixture.state().pendingFactoryOperationIDs).not.toContain(
      "new-confirm-success",
    );
    fireEvent.click(
      screen.getByRole("button", { name: messages.newFactoryCancelLabel }),
    );
    expect(fixture.state().completedFactoryOperationIDs).toEqual([
      "new-validation-success",
    ]);
    expect(
      screen.queryByText("/workspace/dashboard-regression-new/factory"),
    ).toBeNull();
    expect(
      screen.getByPlaceholderText(messages.newFactoryFolderFieldPlaceholder),
    ).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: messages.closeDialogLabel }),
    );
    await waitFor(() => {
      expect(screen.getAllByRole("tab")).toHaveLength(2);
    });
  });

  it("rejects New Factory validation when the target already exists", async () => {
    const fixture = createDashboardRegressionFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const queryClient = createQueryClient();

    renderWithQueryClient(queryClient);
    await waitFor(() => {
      expect(fixture.state().pendingSessionListIDs).toContain("initial");
    });
    await act(async () => {
      fixture.sessionLists.resolve("initial");
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "secondary" })).toBeTruthy();
    });

    const messages = getHeaderControlsMessages("en");
    fireEvent.click(
      screen.getByRole("button", { name: messages.newFactoryButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.newFactoryFolderFieldPlaceholder),
      { target: { value: "/workspace/dashboard-regression" } },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );
    await waitFor(() => {
      expect(fixture.state().pendingFactoryOperationIDs).toContain(
        "open-validation-success",
      );
    });
    await act(async () => {
      fixture.factoryJourneys.resolve("open-validation-success");
    });

    expect(
      await screen.findByText(messages.newFactoryExistingTargetError),
    ).toBeTruthy();
    expect(fixture.state().pendingFactoryOperationIDs).not.toContain(
      "new-confirm-success",
    );
    fireEvent.click(
      screen.getByRole("button", { name: messages.closeDialogLabel }),
    );
    await waitFor(() => {
      expect(screen.getAllByRole("tab")).toHaveLength(2);
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
