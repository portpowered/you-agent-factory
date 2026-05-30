import {
  act,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { DashboardSnapshot } from "./api/dashboard";
import type { FactorySessionSummary } from "./api/factory-sessions/api";
import { DEFAULT_FACTORY_SESSION_ID } from "./api/session-routing";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { sessionStreamToggleLabel } from "./features/header/lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "./features/header/messages/header-controls";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "./features/dashboard/state/dashboardStreamStore";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";
import {
  MockEventSource,
  activeSnapshot,
  registerAppDashboardTestLifecycle,
  renderApp,
  renderAppWithDashboardShell,
} from "./testing/app-shell-test-utils";
import { selectedTickTimelineEvents } from "./testing/app-shell-timeline-test-utils";

const rootFactorySession: FactorySessionSummary = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: DEFAULT_FACTORY_SESSION_ID,
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

function requireEventStream(): MockEventSource {
  const stream = MockEventSource.instances.at(-1);

  if (!stream) {
    throw new Error("expected factory event stream to be opened");
  }

  return stream;
}

function resetTimelineForInitialStreamLoad(): void {
  useFactoryTimelineStore.setState({
    events: [],
    latestTick: 0,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: 0,
    worldViewCache: {},
  });
  useDashboardStreamStore.setState({
    streamState: createDefaultDashboardStreamState(),
  });
}

function emitTimelineMessages(stream: MockEventSource, events = selectedTickTimelineEvents): void {
  for (const event of events) {
    stream.emit("message", event);
  }
}

function buildBetaSessionSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.tick_count = 108;
  snapshot.runtime.session.completed_count = 3;
  snapshot.runtime.session.dispatched_count = 5;
  snapshot.runtime.session.failed_count = 2;

  const renameWorkItem = (
    workItem: NonNullable<
      DashboardSnapshot["runtime"]["current_work_items_by_place_id"]
    >[string][number],
  ) => ({
    ...workItem,
    display_name:
      workItem.display_name === "Active Story"
        ? "Beta Story"
        : workItem.display_name,
    trace_id:
      workItem.trace_id === "trace-active-story"
        ? "trace-beta-story"
        : workItem.trace_id,
    work_id:
      workItem.work_id === "work-active-story"
        ? "work-beta-story"
        : workItem.work_id,
  });

  snapshot.runtime.current_work_items_by_place_id = Object.fromEntries(
    Object.entries(
      snapshot.runtime.current_work_items_by_place_id ?? {},
    ).map(([placeID, workItems]) => [
      placeID,
      workItems?.map(renameWorkItem),
    ]),
  );

  return snapshot;
}

describe("App dashboard session stream shell", () => {
  registerAppDashboardTestLifecycle();

  it("shows the loading shell before the first streamed event, then clears after the first message", async () => {
    const messages = getHeaderControlsMessages("en");

    renderApp({
      seedTimelineFromSnapshot: false,
      snapshot: activeSnapshot,
    });
    resetTimelineForInitialStreamLoad();

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBeGreaterThan(0);
    });

    expect(
      screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("slider", { name: "Timeline tick" }),
    ).toBeNull();

    act(() => {
      emitTimelineMessages(requireEventStream(), [selectedTickTimelineEvents[0]]);
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(
        screen.queryByRole("heading", { name: messages.loadingDashboardTitle }),
      ).toBeNull();
      expect(slider.value).toBe("1");
    });
  });

  it("retargets the live stream URL on session tab switches and closes the prior connection", async () => {
    const betaSnapshot = buildBetaSessionSnapshot();

    renderApp({
      factorySessions: [rootFactorySession, betaFactorySession],
      snapshot: activeSnapshot,
    });

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBeGreaterThan(0);
    });
    await screen.findByRole("tab", { name: "beta" });

    const defaultStream = requireEventStream();
    expect(defaultStream.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );

    const defaultSlider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(defaultSlider.value).toBe(String(activeSnapshot.tick_count));
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

    const betaStream = requireEventStream();
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

  it("stops the live stream when a non-default session is paused and reopens it on resume", async () => {
    const messages = getHeaderControlsMessages("en");
    const betaSnapshot = buildBetaSessionSnapshot();

    await renderAppWithDashboardShell({
      factorySessions: [rootFactorySession, betaFactorySession],
      snapshot: activeSnapshot,
    });

    await screen.findByRole("tab", { name: "beta" });
    fireEvent.click(screen.getByRole("tab", { name: "beta" }));

    await waitFor(() => {
      expect(requireEventStream().url).toBe(
        "/factory-sessions/session-beta/events",
      );
    });

    const liveStream = requireEventStream();

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
      expect(
        useDashboardStreamStore.getState().streamState.message,
      ).toBe("Live session updates paused. Showing last event state.");
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

    const resumedStream = requireEventStream();
    expect(resumedStream.closed).toBe(false);
    expect(resumedStream.url).toBe("/factory-sessions/session-beta/events");
  });
});
