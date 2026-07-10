import "./testing/app-shell-work-outcome-stub";
import "./testing/app-shell-workflow-activity-stub";

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { requireEventStream } from "./App.session-stream.test-helpers";
import type { DashboardSnapshot } from "./api/dashboard";
import type { FactorySessionSummary } from "./api/factory-sessions/api";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import {
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
import {
  MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO,
  type MultiSessionTimelineCheckpointFixture,
} from "./testing/multi-session-timeline-checkpoint-scenario";
import { createTimelineCheckpointIndexedDBTestDouble } from "./testing/timeline-checkpoint-indexeddb-test-utils";

const CHECKPOINT_DEBOUNCE_MS = 750;
const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;

const factorySessions: FactorySessionSummary[] = [
  sessionSummary(A, "alpha"),
  sessionSummary(B, "beta"),
];

function sessionSummary(
  fixture: MultiSessionTimelineCheckpointFixture,
  name: string,
): FactorySessionSummary {
  return {
    factoryDir: `/workspace/${name}`,
    folderPath: `/workspace/${name}`,
    id: fixture.streamIdentity.factorySessionID,
    isDefault: false,
    project: name,
    target: { kind: "named", name },
  };
}

function snapshotFor(
  fixture: MultiSessionTimelineCheckpointFixture,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.tick_count = fixture.checkpoint.selectedTick;
  snapshot.runtime.session.dispatched_count = fixture.eventCount;
  return snapshot;
}

function multiSessionPreflightFetch(): RenderAppFetchOverride {
  return async (path, method) => {
    if (method !== "GET") {
      return undefined;
    }
    const match = path.match(/^\/factory-sessions\/([^/]+)\/sync-preflight/);
    if (match) {
      const sessionID = decodeURIComponent(match[1] ?? "");
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

function expectedStreamURL(
  fixture: MultiSessionTimelineCheckpointFixture,
): string {
  const query = new URLSearchParams({
    after_event_id: fixture.checkpoint.afterEventId ?? "",
    after_sequence: String(fixture.checkpoint.afterSequence),
  });
  return `/factory-sessions/${fixture.streamIdentity.factorySessionID}/events?${query}`;
}

async function flushFakeTimerEffects(): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(0);
    await Promise.resolve();
  });
}

async function selectSession(
  name: "alpha" | "beta",
  fixture: MultiSessionTimelineCheckpointFixture,
): Promise<void> {
  fireEvent.click(screen.getByRole("tab", { name }));
  for (let attempt = 0; attempt < 8; attempt += 1) {
    await flushFakeTimerEffects();
    if (MockEventSource.instances.at(-1)?.url === expectedStreamURL(fixture)) {
      return;
    }
  }
  expect(requireEventStream(MockEventSource.instances).url).toBe(
    expectedStreamURL(fixture),
  );
}

async function scheduleCheckpoint(
  fixture: MultiSessionTimelineCheckpointFixture,
): Promise<void> {
  act(() => {
    useFactoryTimelineStore.getState().restoreCheckpoint(fixture.checkpoint);
  });
  await flushFakeTimerEffects();
  expect(useFactoryTimelineStore.getState().currentReplayCheckpoint).toEqual(
    fixture.checkpoint,
  );
}

async function advanceCheckpointTimer(milliseconds: number): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(milliseconds);
    await Promise.resolve();
  });
}

describe("App multi-session checkpoint debounce regression", () => {
  registerAppDashboardTestLifecycle();

  afterEach(() => {
    vi.useRealTimers();
  });

  async function renderSessionA(): Promise<IDBFactory> {
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    vi.stubGlobal("indexedDB", indexedDB);
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
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    return indexedDB;
  }

  it.fails("retains checkpoints that become due after A to B to A switching just before 750 ms", async () => {
    const indexedDB = await renderSessionA();

    await scheduleCheckpoint(A);
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS - 1);
    await selectSession("beta", B);
    await scheduleCheckpoint(B);
    await selectSession("alpha", A);
    await advanceCheckpointTimer(1);

    const aAtOriginalDueBoundary = await readTimelineCheckpoint(
      indexedDB,
      A.streamIdentity,
    );

    await scheduleCheckpoint(A);
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS);
    await selectSession("beta", B);
    await scheduleCheckpoint(B);
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS);

    expect.soft(aAtOriginalDueBoundary).toEqual(A.checkpoint);
    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);
    await expect(
      readTimelineCheckpoint(indexedDB, B.streamIdentity),
    ).resolves.toEqual(B.checkpoint);
  });

  it("keeps A durable when switching A to B to A after its 750 ms boundary", async () => {
    const indexedDB = await renderSessionA();

    await scheduleCheckpoint(A);
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS);
    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);

    await selectSession("beta", B);
    await scheduleCheckpoint(B);
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS);
    await selectSession("alpha", A);

    expect(requireEventStream(MockEventSource.instances).url).toBe(
      expectedStreamURL(A),
    );
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(
      A.checkpoint.selectedTick,
    );
    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);
    await expect(
      readTimelineCheckpoint(indexedDB, B.streamIdentity),
    ).resolves.toEqual(B.checkpoint);
  });
});
