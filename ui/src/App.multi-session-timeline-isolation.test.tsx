import "./testing/app-shell-work-outcome-stub";
import "./testing/app-shell-workflow-activity-stub";

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { requireEventStream } from "./App.session-stream.test-helpers";
import type { DashboardSnapshot } from "./api/dashboard";
import type { FactoryEvent } from "./api/events";
import type { FactorySessionSummary } from "./api/factory-sessions/api";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import { sessionStreamToggleLabel } from "./features/header/lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "./features/header/messages/header-controls";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  useFactoryTimelineStore,
} from "./features/timeline/public";
import {
  jsonResponse,
  MockEventSource,
  type RenderAppFetchOverride,
  registerAppDashboardTestLifecycle,
  renderAppWithDashboardShell,
} from "./testing/app-shell-test-utils";
import { MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO } from "./testing/multi-session-timeline-checkpoint-scenario";
import { createTimelineCheckpointIndexedDBTestDouble } from "./testing/timeline-checkpoint-indexeddb-test-utils";

const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;

const factorySessions: FactorySessionSummary[] = [
  sessionSummary(A.streamIdentity.factorySessionID, "alpha"),
  sessionSummary(B.streamIdentity.factorySessionID, "beta"),
];

function sessionSummary(id: string, name: string): FactorySessionSummary {
  return {
    factoryDir: `/workspace/${name}`,
    folderPath: `/workspace/${name}`,
    id,
    isDefault: false,
    project: name,
    target: { kind: "named", name },
  };
}

function snapshotFor(fixture: typeof A | typeof B): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.tick_count = fixture.checkpoint.selectedTick;
  snapshot.runtime.session.dispatched_count = fixture.eventCount;
  return snapshot;
}

function liveEventFor(fixture: typeof A | typeof B): FactoryEvent {
  const sequence = (fixture.checkpoint.afterSequence ?? 0) + 1;
  return {
    context: {
      dispatchId: `live-dispatch-${fixture.label.toLowerCase()}`,
      eventTime: "2026-07-20T08:30:00Z",
      sequence,
      sessionId: fixture.streamIdentity.factorySessionID,
      sessionSequence: sequence,
      tick: fixture.checkpoint.selectedTick + 1,
    },
    id: `live-event-${fixture.label.toLowerCase()}`,
    payload: {
      inputs: [],
      resources: [],
      transitionId: "live-review",
    },
    schemaVersion: "agent-factory.event.v1",
    type: "DISPATCH_REQUEST",
  };
}

function multiSessionPreflightFetch(): RenderAppFetchOverride {
  return async (path, method) => {
    if (
      method === "DELETE" &&
      path === `/factory-sessions/${A.streamIdentity.factorySessionID}`
    ) {
      return new Response(null, { status: 204 });
    }
    if (method !== "GET") {
      return undefined;
    }

    const preflightMatch = path.match(
      /^\/factory-sessions\/([^/]+)\/sync-preflight/,
    );
    if (preflightMatch) {
      const sessionID = decodeURIComponent(preflightMatch[1] ?? "");
      const fixture = sessionID === A.streamIdentity.factorySessionID ? A : B;
      return jsonResponse({
        backendScopeId: fixture.streamIdentity.backendScopeID,
        checkpointReusable: true,
        factorySessionId: fixture.streamIdentity.factorySessionID,
        logicalSessionKeyId: fixture.streamIdentity.logicalSessionKeyID,
        reasonCode: "ok",
        reconnectCursor: {
          afterEventId: fixture.checkpoint.afterEventId,
          afterSequence: fixture.checkpoint.afterSequence,
          provided: true,
          validForStreamGeneration: true,
        },
        requestedSessionId: sessionID,
        streamGenerationId: fixture.streamIdentity.streamGenerationID,
      });
    }

    if (/^\/factory-sessions\/[^/]+\/events\?/.test(path)) {
      return new Response("event: ready\n\n", { status: 200 });
    }
    return undefined;
  };
}

function expectedStreamURL(fixture: typeof A | typeof B): string {
  const query = new URLSearchParams({
    after_event_id: fixture.checkpoint.afterEventId ?? "",
    after_sequence: String(fixture.checkpoint.afterSequence),
  });
  return `/factory-sessions/${fixture.streamIdentity.factorySessionID}/events?${query}`;
}

async function selectSessionTab(name: "alpha" | "beta"): Promise<void> {
  fireEvent.click(await screen.findByRole("tab", { name }));
}

function expectRenderedFixture(fixture: typeof A | typeof B): void {
  const timeline = useFactoryTimelineStore.getState();
  const exactEntry = timeline.entryForIdentity(fixture.streamIdentity);
  expect(exactEntry?.selectedTick).toBe(fixture.checkpoint.selectedTick);
  expect(
    exactEntry?.currentReplayCheckpoint?.replayState.runtime.session
      .dispatched_count,
  ).toBe(fixture.eventCount);
  expect(
    screen.getByRole<HTMLInputElement>("slider", { name: "Timeline tick" })
      .value,
  ).toBe(String(fixture.checkpoint.selectedTick));
}

function expectRenderedLiveEvent(fixture: typeof A | typeof B): void {
  const event = liveEventFor(fixture);
  const exactEntry = useFactoryTimelineStore
    .getState()
    .entryForIdentity(fixture.streamIdentity);
  expect(exactEntry?.selectedTick).toBe(event.context.tick);
  expect(exactEntry?.currentReplayCheckpoint?.selectedTick).toBe(
    event.context.tick,
  );
  expect(exactEntry?.currentReplayCheckpoint?.afterEventId).toBe(event.id);
  expect(
    screen.getByRole<HTMLInputElement>("slider", { name: "Timeline tick" })
      .value,
  ).toBe(String(event.context.tick));
}

describe("App multi-session timeline switching regression", () => {
  registerAppDashboardTestLifecycle();

  it("restores each live A to B to A timeline instead of retaining the singleton's latest contents", async () => {
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    vi.stubGlobal("indexedDB", indexedDB);
    await persistTimelineCheckpoint(indexedDB, A.checkpoint, A.streamIdentity);
    await persistTimelineCheckpoint(indexedDB, B.checkpoint, B.streamIdentity);
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: A.streamIdentity.factorySessionID,
    });

    await renderAppWithDashboardShell({
      factorySessions,
      fetchOverride: multiSessionPreflightFetch(),
      seedTimelineFromSnapshot: false,
      snapshot: snapshotFor(A),
    });

    await waitFor(() => {
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        expectedStreamURL(A),
      );
    });
    act(() => {
      const stream = requireEventStream(MockEventSource.instances);
      stream.emit("snapshot", snapshotFor(A));
      stream.emit("message", liveEventFor(A));
    });
    await waitFor(() => expectRenderedLiveEvent(A));

    await selectSessionTab("beta");
    await waitFor(() => {
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        expectedStreamURL(B),
      );
    });
    act(() => {
      const stream = requireEventStream(MockEventSource.instances);
      stream.emit("snapshot", snapshotFor(B));
      stream.emit("message", liveEventFor(B));
    });
    await waitFor(() => expectRenderedLiveEvent(B));

    await selectSessionTab("alpha");
    await waitFor(() => {
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        expectedStreamURL(A),
      );
      expectRenderedLiveEvent(A);
    });
  });
});

describe("App multi-session timeline action isolation", () => {
  registerAppDashboardTestLifecycle();

  it("pauses only A while B keeps its persisted timeline, cursor, and live stream", async () => {
    const messages = getHeaderControlsMessages("en");
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    vi.stubGlobal("indexedDB", indexedDB);
    await persistTimelineCheckpoint(indexedDB, A.checkpoint, A.streamIdentity);
    await persistTimelineCheckpoint(indexedDB, B.checkpoint, B.streamIdentity);
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: A.streamIdentity.factorySessionID,
    });

    await renderAppWithDashboardShell({
      factorySessions,
      fetchOverride: multiSessionPreflightFetch(),
      seedTimelineFromSnapshot: false,
      snapshot: snapshotFor(A),
    });
    await waitFor(() => expectRenderedFixture(A));
    const aStream = requireEventStream(MockEventSource.instances);

    fireEvent.click(
      screen.getByRole("button", {
        name: sessionStreamToggleLabel(factorySessions[0], false, messages),
      }),
    );
    await waitFor(() => {
      expect(aStream.closed).toBe(true);
      expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([
        A.streamIdentity.factorySessionID,
      ]);
    });

    await selectSessionTab("beta");
    await waitFor(() => {
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        expectedStreamURL(B),
      );
      expectRenderedFixture(B);
    });
    expect(requireEventStream(MockEventSource.instances).closed).toBe(false);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).not.toContain(
      B.streamIdentity.factorySessionID,
    );
    await expect(
      readTimelineCheckpoint(indexedDB, B.streamIdentity),
    ).resolves.toEqual(B.checkpoint);
  });

  it("clears only A while B keeps its timeline, cursor, checkpoint, and live stream", async () => {
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    vi.stubGlobal("indexedDB", indexedDB);
    await persistTimelineCheckpoint(indexedDB, A.checkpoint, A.streamIdentity);
    await persistTimelineCheckpoint(indexedDB, B.checkpoint, B.streamIdentity);
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: A.streamIdentity.factorySessionID,
    });

    await renderAppWithDashboardShell({
      factorySessions,
      fetchOverride: multiSessionPreflightFetch(),
      seedTimelineFromSnapshot: false,
      snapshot: snapshotFor(A),
    });
    await waitFor(() => expectRenderedFixture(A));
    const aStream = requireEventStream(MockEventSource.instances);

    fireEvent.click(
      screen.getByRole("button", { name: "Close alpha session" }),
    );

    await waitFor(() => {
      expect(aStream.closed).toBe(true);
      expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
        B.streamIdentity.factorySessionID,
      );
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        expectedStreamURL(B),
      );
      expectRenderedFixture(B);
    });
    expect(requireEventStream(MockEventSource.instances).closed).toBe(false);
    await expect(
      readTimelineCheckpoint(indexedDB, B.streamIdentity),
    ).resolves.toEqual(B.checkpoint);
    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toBe(null);
    expect(
      useFactoryTimelineStore.getState().entryForIdentity(A.streamIdentity),
    ).toBeUndefined();
  });
});
