import "../../../../testing/app-shell-work-outcome-stub";
import "../../../../testing/app-shell-workflow-activity-stub";

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DashboardSnapshot } from "../../../../api/dashboard";
import type { FactorySessionSummary } from "../../../../api/factory-sessions/api";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import {
  jsonResponse,
  MockEventSource,
  type RenderAppFetchOverride,
  registerAppDashboardTestLifecycle,
} from "../../../../testing/app-shell-test-utils";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import {
  MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO,
  type MultiSessionTimelineCheckpointFixture,
} from "../../../../testing/multi-session-timeline-checkpoint-scenario";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import {
  readTimelineCheckpoint,
} from "../../../timeline/public/checkpoint-persistence";
import { useFactoryTimelineStore } from "../../../timeline/public/store";
import { requireEventStream } from "../../session/dashboard-session-stream-test-helpers";
import { useDashboardSessionStore } from "../../state/dashboardSessionStore";
import { renderDashboardScreenWithShell as renderAppWithDashboardShell } from "../../components/testing/dashboard-screen-test-render";

const CHECKPOINT_DEBOUNCE_MS = 750;
const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;

interface StoredCheckpointEnvelope {
  checkpoint?: MultiSessionTimelineCheckpointFixture["checkpoint"];
  schemaVersion?: number;
  storageKey?: string;
  streamIdentity?: MultiSessionTimelineCheckpointFixture["streamIdentity"];
}

function previousCheckpointFor(
  fixture: MultiSessionTimelineCheckpointFixture,
): MultiSessionTimelineCheckpointFixture {
  const previous = structuredClone(fixture);
  previous.checkpoint.afterEventId = `session-${fixture.label.toLowerCase()}-event-previous`;
  previous.checkpoint.afterSequence -= 1;
  previous.checkpoint.selectedTick -= 1;
  previous.checkpoint.replayState.tick_count -= 1;
  if (previous.checkpoint.materializedWorkOutcomeState.cursor) {
    previous.checkpoint.materializedWorkOutcomeState.cursor.eventID =
      previous.checkpoint.afterEventId;
    previous.checkpoint.materializedWorkOutcomeState.cursor.sequence =
      previous.checkpoint.afterSequence;
    previous.checkpoint.materializedWorkOutcomeState.cursor.tick =
      previous.checkpoint.selectedTick;
  }
  return previous;
}

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

async function settleCheckpointPersistence(): Promise<void> {
  await act(async () => {
    for (let turn = 0; turn < 8; turn += 1) {
      await Promise.resolve();
    }
  });
}

async function waitForControlledOperation(
  fixture: ReturnType<
    typeof createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>
  >,
  operation: "get" | "open" | "put",
): Promise<void> {
  for (let turn = 0; turn < 32; turn += 1) {
    if (fixture.controls.pendingOperations().includes(operation)) return;
    await flushPromiseContinuations();
  }
  expect(fixture.controls.pendingOperations()).toContain(operation);
}

async function advanceControlledWrite(
  fixture: ReturnType<
    typeof createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>
  >,
  openOrdinal = 0,
): Promise<void> {
  fixture.controls.succeed("open", openOrdinal);
  await waitForControlledOperation(fixture, "get");
  fixture.controls.succeed("get");
  await flushPromiseContinuations();
  if (fixture.controls.pendingOperations().includes("put")) {
    await waitForControlledOperation(fixture, "put");
    fixture.controls.succeed("put");
  }
  fixture.controls.completeTransaction();
  for (let turn = 0; turn < 6; turn += 1) {
    await flushPromiseContinuations();
  }
}

async function renderSessionA(options: { writeError?: Error } = {}) {
  const checkpointDatabase =
    createTimelineCheckpointIndexedDBTestDouble(options);
  const { indexedDB } = checkpointDatabase;
  vi.stubGlobal("indexedDB", indexedDB);
  useDashboardSessionStore.setState({
    pausedSessionIDs: [],
    selectedSessionID: A.streamIdentity.factorySessionID,
  });
  const view = await renderAppWithDashboardShell({
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
  return { ...checkpointDatabase, ...view };
}

describe("dashboard multi-session checkpoint debounce", () => {
  registerAppDashboardTestLifecycle();

  afterEach(() => {
    vi.useRealTimers();
  });

  it("flushes each stream's latest checkpoint during rapid A to B to A switching", async () => {
    const { indexedDB } = await renderSessionA();

    await scheduleCheckpoint(previousCheckpointFor(A));
    await scheduleCheckpoint(A);
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS - 1);
    await selectSession("beta", B);

    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);

    await scheduleCheckpoint(B);
    await selectSession("alpha", A);

    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);
    await expect(
      readTimelineCheckpoint(indexedDB, B.streamIdentity),
    ).resolves.toEqual(B.checkpoint);
  });

  it("keeps A durable when switching A to B to A after its 750 ms boundary", async () => {
    const { indexedDB } = await renderSessionA();

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

describe("dashboard checkpoint lifecycle handoff", () => {
  registerAppDashboardTestLifecycle();

  afterEach(() => {
    vi.useRealTimers();
  });

  it("flushes the latest pending checkpoint on unmount", async () => {
    const { indexedDB, unmount, writeAttempts } = await renderSessionA();

    await scheduleCheckpoint(previousCheckpointFor(A));
    await scheduleCheckpoint(A);
    unmount();
    await settleCheckpointPersistence();

    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);
    expect(writeAttempts()).toBe(1);
  });

  it("flushes on pagehide without blocking navigation or repeating the handoff", async () => {
    const { indexedDB, writeAttempts } = await renderSessionA();
    await scheduleCheckpoint(A);
    const pageHide = new Event("pagehide", { cancelable: true });

    expect(window.dispatchEvent(pageHide)).toBe(true);
    expect(pageHide.defaultPrevented).toBe(false);
    window.dispatchEvent(new Event("pagehide"));
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS);

    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);
    expect(writeAttempts()).toBe(1);
  });

  it("flushes only when visibility changes to hidden", async () => {
    const { indexedDB, writeAttempts } = await renderSessionA();
    await scheduleCheckpoint(A);
    const visibilityState = vi.spyOn(document, "visibilityState", "get");

    visibilityState.mockReturnValue("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    visibilityState.mockReturnValue("prerender");
    document.dispatchEvent(new Event("visibilitychange"));
    expect(writeAttempts()).toBe(0);

    visibilityState.mockReturnValue("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    document.dispatchEvent(new Event("visibilitychange"));
    await settleCheckpointPersistence();

    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toEqual(A.checkpoint);
    expect(writeAttempts()).toBe(1);
  });
});

describe("dashboard checkpoint lifecycle safety", () => {
  registerAppDashboardTestLifecycle();

  afterEach(() => {
    vi.useRealTimers();
  });

  it("installs lifecycle listeners once and removes them on unmount", async () => {
    const addWindowListener = vi.spyOn(window, "addEventListener");
    const removeWindowListener = vi.spyOn(window, "removeEventListener");
    const addDocumentListener = vi.spyOn(document, "addEventListener");
    const removeDocumentListener = vi.spyOn(document, "removeEventListener");
    const { unmount } = await renderSessionA();

    expect(
      addWindowListener.mock.calls.filter(([type]) => type === "pagehide"),
    ).toHaveLength(1);
    expect(
      addDocumentListener.mock.calls.filter(
        ([type]) => type === "visibilitychange",
      ),
    ).toHaveLength(1);

    unmount();

    expect(
      removeWindowListener.mock.calls.filter(([type]) => type === "pagehide"),
    ).toHaveLength(1);
    expect(
      removeDocumentListener.mock.calls.filter(
        ([type]) => type === "visibilitychange",
      ),
    ).toHaveLength(1);
  });

  it("contains a lifecycle-time persistence rejection", async () => {
    const { indexedDB, writeAttempts } = await renderSessionA({
      writeError: new Error("storage refused"),
    });
    await scheduleCheckpoint(A);
    const pageHide = new Event("pagehide", { cancelable: true });

    expect(window.dispatchEvent(pageHide)).toBe(true);
    await settleCheckpointPersistence();

    await expect(
      readTimelineCheckpoint(indexedDB, A.streamIdentity),
    ).resolves.toBeNull();
    expect(pageHide.defaultPrevented).toBe(false);
    expect(writeAttempts()).toBe(1);
  });

  it("keeps newer same-stream state authoritative while A and B lifecycle writes overlap", async () => {
    const { indexedDB: preflightIndexedDB, unmount } = await renderSessionA();
    const controlled =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    vi.stubGlobal("indexedDB", controlled.indexedDB);

    await scheduleCheckpoint(previousCheckpointFor(A));
    window.dispatchEvent(new Event("pagehide"));
    await flushPromiseContinuations();
    await scheduleCheckpoint(A);
    await advanceCheckpointTimer(CHECKPOINT_DEBOUNCE_MS);

    vi.stubGlobal("indexedDB", preflightIndexedDB);
    await selectSession("beta", B);
    vi.stubGlobal("indexedDB", controlled.indexedDB);
    await scheduleCheckpoint(B);
    window.dispatchEvent(new Event("pagehide"));
    for (let turn = 0; turn < 12; turn += 1) {
      await flushPromiseContinuations();
    }

    expect(controlled.controls.pendingOperations()).toEqual(["open", "open"]);

    // B owns an independent identity lane and may finish before the older A
    // lifecycle write even though A was admitted first.
    await advanceControlledWrite(controlled, 1);
    await advanceControlledWrite(controlled);
    expect(controlled.controls.pendingOperations()).toEqual(["open"]);
    await advanceControlledWrite(controlled);

    const stored = [...controlled.records.values()];
    expect(stored).toHaveLength(2);
    expect(
      stored.find(
        ({ streamIdentity }) =>
          streamIdentity?.factorySessionID ===
          A.streamIdentity.factorySessionID,
      ),
    ).toEqual({
      checkpoint: A.checkpoint,
      schemaVersion: expect.any(Number),
      storageKey: expect.any(String),
      streamIdentity: A.streamIdentity,
    });
    expect(
      stored.find(
        ({ streamIdentity }) =>
          streamIdentity?.factorySessionID ===
          B.streamIdentity.factorySessionID,
      ),
    ).toEqual({
      checkpoint: B.checkpoint,
      schemaVersion: expect.any(Number),
      storageKey: expect.any(String),
      streamIdentity: B.streamIdentity,
    });

    unmount();
    expect(controlled.controls.pendingOperations()).toEqual([]);
  });
});
