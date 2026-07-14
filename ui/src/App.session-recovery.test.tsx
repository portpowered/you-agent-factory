import "./testing/app-shell-workflow-activity-stub";

import { act, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { requireEventStream } from "./App.session-stream.test-helpers";
import type { DashboardSnapshot } from "./api/dashboard";
import { FACTORY_EVENT_TYPES, type FactoryEvent } from "./api/events";
import type { FactorySessionSummary } from "./api/factory-sessions/api";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  useFactoryTimelineStore,
} from "./features/timeline/public";
import { emptyReplayWorldState } from "./features/timeline/state/timeline/replayWorldStateSupport";
import {
  createMaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
} from "./features/work-outcome/public/materializer";
import {
  buildAppShellStreamIdentity,
  buildFactorySessionResponse,
} from "./testing/app-shell-session-preflight-test-utils";
import {
  jsonResponse,
  MockEventSource,
  type RenderAppFetchOverride,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";
import { createTimelineCheckpointIndexedDBTestDouble } from "./testing/timeline-checkpoint-indexeddb-test-utils";

const ALPHA_SESSION_ID = "11111111-1111-4111-8111-111111111111";
const BETA_SESSION_ID = "22222222-2222-4222-8222-222222222222";
const STALE_SESSION_ID = "33333333-3333-4333-8333-333333333333";
const REMAPPED_SESSION_ID = "44444444-4444-4444-8444-444444444444";

const alphaSession = sessionSummary(ALPHA_SESSION_ID, "alpha");
const betaSession = sessionSummary(BETA_SESSION_ID, "beta");

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

function factoryEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-13T12:00:${String(tick).padStart(2, "0")}Z`,
      sequence: tick,
      sessionSequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

function workRequestEvent(tick: number): FactoryEvent {
  return factoryEvent(
    `alpha-work-request-${tick}`,
    tick,
    FACTORY_EVENT_TYPES.workRequest,
    {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: `Alpha Story ${tick}`,
          traceId: `alpha-trace-${tick}`,
          workId: `alpha-work-${tick}`,
          workTypeName: "story",
        },
      ],
    },
  );
}

const baselineEvents = [
  factoryEvent("alpha-run-started", 0, FACTORY_EVENT_TYPES.runRequest, {
    factory: {
      resources: [],
      workers: [],
      workstations: [],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
    },
    recordedAt: "2026-07-13T12:00:00Z",
  }),
  ...Array.from({ length: 6 }, (_, index) => workRequestEvent(index + 1)),
] satisfies FactoryEvent[];

const tailEvent = workRequestEvent(7);

function snapshotAt(tick: number): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.tick_count = tick;
  return snapshot;
}

function checkpointFor(
  session: FactorySessionSummary,
  snapshot: DashboardSnapshot,
  events: FactoryEvent[],
) {
  const streamIdentity = buildAppShellStreamIdentity(session, snapshot);
  const lastEvent = events.at(-1);
  const replayState = emptyReplayWorldState(lastEvent?.context.tick ?? 0);
  replayState.factory_state = `${session.project} restored`;
  replayState.runtime.session.has_data = true;
  const materializedWorkOutcomeState = reduceMaterializedWorkOutcomeEvents(
    createMaterializedWorkOutcomeState(),
    events,
  );
  return {
    checkpoint: {
      afterEventId: lastEvent?.id,
      afterSequence: lastEvent?.context.sessionSequence,
      materializedWorkOutcomeState,
      replayState,
      selectedTick: lastEvent?.context.tick ?? 0,
      syncIdentity: {
        backendScopeId: streamIdentity.backendScopeID,
        factorySessionId: streamIdentity.factorySessionID,
        logicalSessionKeyId: streamIdentity.logicalSessionKeyID,
        streamGenerationId: streamIdentity.streamGenerationID,
      },
    },
    streamIdentity,
  };
}

function expectedStreamURL(
  sessionID: string,
  afterEventID?: string,
  afterSequence?: number,
): string {
  const query = new URLSearchParams();
  if (afterEventID) query.set("after_event_id", afterEventID);
  if (afterSequence != null) query.set("after_sequence", String(afterSequence));
  const suffix = query.size > 0 ? `?${query}` : "";
  return `/factory-sessions/${sessionID}/events${suffix}`;
}

describe("App materialized session recovery composition", () => {
  registerAppDashboardTestLifecycle();

  it("renders six restored outcomes before one uniquely applied live tail", async () => {
    const snapshot = snapshotAt(6);
    const alpha = checkpointFor(alphaSession, snapshot, baselineEvents);
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    vi.stubGlobal("indexedDB", indexedDB);
    await persistTimelineCheckpoint(
      indexedDB,
      alpha.checkpoint,
      alpha.streamIdentity,
    );
    useDashboardSessionStore.setState({ selectedSessionID: ALPHA_SESSION_ID });

    renderApp({
      factorySessions: [alphaSession, betaSession],
      seedTimelineFromSnapshot: false,
      snapshot,
    });

    await waitFor(() => {
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        expectedStreamURL(
          ALPHA_SESSION_ID,
          alpha.checkpoint.afterEventId,
          alpha.checkpoint.afterSequence,
        ),
      );
    });
    const chart = await screen.findByRole("article", {
      name: "Work outcome chart",
    });
    const chartRoot = within(chart).getByRole("img", {
      name: "Work outcome chart for Session",
    });
    expect(chartRoot.getAttribute("data-work-chart-visible-ticks")).toBe(
      "0,1,2,3,4,5,6",
    );
    expect(
      requireEventStream(MockEventSource.instances).messageEventCount,
    ).toBe(0);

    const stream = requireEventStream(MockEventSource.instances);
    await act(async () => {
      stream.emit("message", tailEvent);
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(chartRoot.getAttribute("data-work-chart-visible-ticks")).toBe(
        "0,1,2,3,4,5,6,7",
      );
    });
    expect(
      useFactoryTimelineStore
        .getState()
        .materializedWorkOutcomeState.samples.map((sample) => sample.tick),
    ).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
    const countsAfterTail = structuredClone(
      useFactoryTimelineStore.getState().materializedWorkOutcomeState.counts,
    );

    await act(async () => {
      stream.emit("message", tailEvent);
      await Promise.resolve();
    });
    expect(
      useFactoryTimelineStore
        .getState()
        .materializedWorkOutcomeState.samples.map((sample) => sample.tick),
    ).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
    expect(
      useFactoryTimelineStore.getState().materializedWorkOutcomeState.counts,
    ).toEqual(countsAfterTail);
    expect(
      useFactoryTimelineStore
        .getState()
        .events.filter((event) => event.id === tailEvent.id),
    ).toHaveLength(1);
  });
});

describe("App logical session remap composition", () => {
  registerAppDashboardTestLifecycle();

  it("remaps only the stale exact session while preserving another session", async () => {
    const snapshot = snapshotAt(6);
    const staleSession = sessionSummary(STALE_SESSION_ID, "alpha");
    const remappedSession = sessionSummary(REMAPPED_SESSION_ID, "alpha");
    const stale = checkpointFor(staleSession, snapshot, baselineEvents);
    const beta = checkpointFor(
      betaSession,
      snapshotAt(3),
      baselineEvents.slice(0, 4),
    );
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    vi.stubGlobal("indexedDB", indexedDB);
    await persistTimelineCheckpoint(
      indexedDB,
      stale.checkpoint,
      stale.streamIdentity,
    );
    await persistTimelineCheckpoint(
      indexedDB,
      beta.checkpoint,
      beta.streamIdentity,
    );
    useDashboardSessionStore.setState({
      pausedSessionIDs: [STALE_SESSION_ID, BETA_SESSION_ID],
      selectedSessionID: STALE_SESSION_ID,
      sessionTabOrder: [STALE_SESSION_ID, BETA_SESSION_ID],
    });

    const remapFetch: RenderAppFetchOverride = async (path, method) => {
      if (method !== "GET") return undefined;
      const preflightMatch = path.match(
        /^\/factory-sessions\/([^/]+)\/sync-preflight/,
      );
      if (preflightMatch) {
        const requestedSessionID = decodeURIComponent(preflightMatch[1] ?? "");
        const isStale = requestedSessionID === STALE_SESSION_ID;
        const identity = buildAppShellStreamIdentity(remappedSession, snapshot);
        return jsonResponse({
          backendScopeId: identity.backendScopeID,
          checkpointReusable: !isStale,
          factorySessionId: REMAPPED_SESSION_ID,
          logicalSessionKeyId: identity.logicalSessionKeyID,
          reasonCode: isStale ? "logical_session_remap" : "ok",
          reconnectCursor: {
            ...(isStale
              ? {
                  afterEventId: stale.checkpoint.afterEventId,
                  afterSequence: stale.checkpoint.afterSequence,
                }
              : {}),
            provided: isStale,
            validForStreamGeneration: !isStale,
          },
          requestedSessionId: requestedSessionID,
          streamGenerationId: identity.streamGenerationID,
        });
      }
      if (path === `/factory-sessions/${REMAPPED_SESSION_ID}`) {
        return jsonResponse(
          buildFactorySessionResponse(remappedSession, snapshot),
        );
      }
      if (path.startsWith(`/factory-sessions/${REMAPPED_SESSION_ID}/events`)) {
        return new Response(null, { status: 200 });
      }
      return undefined;
    };

    renderApp({
      factorySessions: [staleSession, betaSession],
      fetchOverride: remapFetch,
      seedTimelineFromSnapshot: false,
      snapshot,
    });

    await waitFor(() => {
      expect(useDashboardSessionStore.getState()).toMatchObject({
        pausedSessionIDs: [BETA_SESSION_ID],
        selectedSessionID: REMAPPED_SESSION_ID,
        sessionTabOrder: [REMAPPED_SESSION_ID, BETA_SESSION_ID],
      });
      expect(requireEventStream(MockEventSource.instances).url).toBe(
        expectedStreamURL(REMAPPED_SESSION_ID),
      );
    });
    await expect(
      readTimelineCheckpoint(indexedDB, stale.streamIdentity),
    ).resolves.toBeNull();
    await expect(
      readTimelineCheckpoint(indexedDB, beta.streamIdentity),
    ).resolves.toEqual(beta.checkpoint);
  });
});
