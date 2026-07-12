import "./testing/app-shell-work-outcome-stub";
import "./testing/app-shell-workflow-activity-stub";

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  buildBetaSessionSnapshot,
  emitTimelineMessagesAct,
  requireEventStream,
  resetTimelineForInitialStreamLoad,
} from "./App.session-stream.test-helpers";
import type { FactorySessionSummary } from "./api/factory-sessions/api";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useDashboardBentoStore } from "./features/bento/state/dashboardBentoStore";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import { useDashboardStreamStore } from "./features/dashboard/state/dashboardStreamStore";
import { sessionStreamToggleLabel } from "./features/header/lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "./features/header/messages/header-controls";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";
import { APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID } from "./testing/app-shell-session-preflight-test-utils";
import {
  MockEventSource,
  registerAppDashboardTestLifecycle,
  renderApp,
  renderAppWithDashboardShell,
  settleAppShellDashboardEffects,
} from "./testing/app-shell-test-utils";
import { selectedTickTimelineEvents } from "./testing/app-shell-timeline-test-utils";
import { createQuietCheckpointReloadFixture } from "./testing/quiet-checkpoint-reload-test-utils";

const rootFactorySession: FactorySessionSummary = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID,
  isDefault: true,
  project: "root",
  target: {
    kind: "default",
  },
};

const betaFactorySession: FactorySessionSummary = {
  factoryDir: "/workspace/root/beta",
  folderPath: "/workspace/root",
  id: "session-beta",
  isDefault: false,
  project: "beta",
  target: {
    kind: "named",
    name: "beta",
  },
};

describe("App dashboard session stream loading", () => {
  registerAppDashboardTestLifecycle();

  it("shows the loading shell before the first streamed event, then clears after the first message", async () => {
    const messages = getHeaderControlsMessages("en");

    resetTimelineForInitialStreamLoad();
    renderApp({
      seedTimelineFromSnapshot: false,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBeGreaterThan(0);
    });
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
      ).toBeTruthy();
    });
    expect(screen.queryByRole("slider", { name: "Timeline tick" })).toBeNull();

    await emitTimelineMessagesAct(
      requireEventStream(MockEventSource.instances),
      [selectedTickTimelineEvents[0]],
    );

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(
        screen.queryByRole("heading", { name: messages.loadingDashboardTitle }),
      ).toBeNull();
      expect(slider.value).toBe("1");
    });
    await settleAppShellDashboardEffects();
  });

  it.fails("renders a restored tick-zero checkpoint after preflight and a quiet stream open", async () => {
    const messages = getHeaderControlsMessages("en");
    const fixture = createQuietCheckpointReloadFixture(0);
    await fixture.installCheckpoint();

    renderApp({
      fetchOverride: fixture.fetchOverride,
      seedTimelineFromSnapshot: false,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    expect(fixture.observations()).toEqual({
      checkpointHydrated: false,
      eventArrived: false,
      preflightCompleted: false,
      streamOpened: false,
    });

    fixture.completePreflight();
    await fixture.waitForCheckpointHydration();
    const stream = await fixture.waitForStreamCreation();

    expect(fixture.observations()).toEqual({
      checkpointHydrated: true,
      eventArrived: false,
      preflightCompleted: true,
      streamOpened: false,
    });

    act(() => {
      fixture.openStream(stream);
    });

    expect(fixture.observations()).toEqual({
      checkpointHydrated: true,
      eventArrived: false,
      preflightCompleted: true,
      streamOpened: true,
    });
    expect(stream.messageEventCount).toBe(0);
    expect(useFactoryTimelineStore.getState()).toMatchObject({
      events: [],
      mode: "current",
      selectedTick: 0,
    });
    expect(
      useFactoryTimelineStore.getState().worldViewCache[0]?.factory_state,
    ).toBe("CHECKPOINT_CURRENT_AT_0");

    await waitFor(() => {
      expect(
        screen.queryByRole("heading", { name: messages.loadingDashboardTitle }),
      ).toBeNull();
      expect(
        screen.getByRole<HTMLInputElement>("slider", {
          name: messages.sliderAriaLabel,
        }).value,
      ).toBe("0");
    });
  });
});

describe("App dashboard session stream tab switch", () => {
  registerAppDashboardTestLifecycle();

  it("retargets the live stream URL on session tab switches and closes the prior connection", async () => {
    const betaSnapshot = buildBetaSessionSnapshot();

    renderApp({
      factorySessions: [rootFactorySession, betaFactorySession],
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBeGreaterThan(0);
    });
    await screen.findByRole("tab", { name: "beta" });

    const defaultStream = requireEventStream(MockEventSource.instances);
    expect(defaultStream.url).toBe(
      `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );
    expect(defaultStream.url).not.toBe("/events");

    const defaultSlider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(defaultSlider.value).toBe(
        String(semanticWorkflowDashboardSnapshot.tick_count),
      );
    });

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBe(2);
      expect(defaultStream.closed).toBe(true);
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
      expect(useFactoryTimelineStore.getState().events).toHaveLength(0);
    });

    const betaStream = requireEventStream(MockEventSource.instances);
    expect(betaStream.url).toBe("/factory-sessions/session-beta/events");

    act(() => {
      betaStream.emit("snapshot", betaSnapshot);
    });

    const betaSlider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(betaSlider.value).toBe(String(betaSnapshot.tick_count));
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(
        betaSnapshot.tick_count,
      );
    });
  });
});

describe("App dashboard session stream pause", () => {
  registerAppDashboardTestLifecycle();

  it("stops the live stream when a non-default session is paused and reopens it on resume", async () => {
    const messages = getHeaderControlsMessages("en");
    const betaSnapshot = buildBetaSessionSnapshot();

    await renderAppWithDashboardShell({
      factorySessions: [rootFactorySession, betaFactorySession],
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    await screen.findByRole("tab", { name: "beta" });
    fireEvent.click(screen.getByRole("tab", { name: "beta" }));

    await waitFor(() => {
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        "/factory-sessions/session-beta/events",
      );
    });

    const liveStream = requireEventStream(MockEventSource.instances);

    act(() => {
      liveStream.emit("snapshot", betaSnapshot);
    });

    await screen.findByRole("slider", { name: "Timeline tick" });

    fireEvent.click(
      screen.getByRole("button", {
        name: sessionStreamToggleLabel(betaFactorySession, false, messages),
      }),
    );

    await waitFor(() => {
      expect(liveStream.closed).toBe(true);
      expect(useDashboardStreamStore.getState().streamState.message).toBe(
        "Live session updates paused. Showing last event state.",
      );
    });

    const streamCountBeforeResume = MockEventSource.instances.length;

    fireEvent.click(
      screen.getByRole("button", {
        name: sessionStreamToggleLabel(betaFactorySession, true, messages),
      }),
    );

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBeGreaterThan(
        streamCountBeforeResume,
      );
    });

    const resumedStream = requireEventStream(MockEventSource.instances);
    expect(resumedStream.closed).toBe(false);
    expect(resumedStream.url).toBe("/factory-sessions/session-beta/events");
  });
});

describe("App dashboard session stream refresh", () => {
  registerAppDashboardTestLifecycle();

  it("shows the loading shell and reopens the stream when the bento refresh token increments", async () => {
    const messages = getHeaderControlsMessages("en");

    resetTimelineForInitialStreamLoad();
    renderApp({
      seedTimelineFromSnapshot: false,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBeGreaterThan(0);
    });

    const initialStream = requireEventStream(MockEventSource.instances);
    await emitTimelineMessagesAct(initialStream, [
      selectedTickTimelineEvents[0],
    ]);

    await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(
        screen.queryByRole("heading", { name: messages.loadingDashboardTitle }),
      ).toBeNull();
    });
    await settleAppShellDashboardEffects();

    const streamCountBeforeRefresh = MockEventSource.instances.length;

    act(() => {
      useDashboardBentoStore.getState().incrementRefreshToken();
    });

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
      ).toBeTruthy();
      expect(initialStream.closed).toBe(true);
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
      expect(MockEventSource.instances.length).toBeGreaterThan(
        streamCountBeforeRefresh,
      );
    });

    const refreshedStream = requireEventStream(MockEventSource.instances);
    expect(refreshedStream.url).toBe(
      `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );

    await emitTimelineMessagesAct(refreshedStream, [
      selectedTickTimelineEvents[0],
    ]);

    await waitFor(() => {
      expect(
        screen.queryByRole("heading", { name: messages.loadingDashboardTitle }),
      ).toBeNull();
    });
    await settleAppShellDashboardEffects();
  });
});
