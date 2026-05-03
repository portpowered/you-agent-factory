import { act, waitFor } from "@testing-library/react";

import type { DashboardSnapshot } from "../api/dashboard";
import type { EventSourceLike } from "../api/events/api";
import type { FactoryEvent } from "../api/events";
import type { WorldState } from "../features/timeline/state/factoryTimelineStore";
import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import { loadReplayFixtureEvents, type ReplayFixtureID } from "./replay-fixtures";

const REPLAY_SETTLE_TIMEOUT_MS = 10_000;
let originalEventSource: typeof window.EventSource | undefined;

export class ReplayEventSource implements EventSourceLike {
  public static instances: ReplayEventSource[] = [];

  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;

  private readonly listeners = new Map<string, EventListener[]>();

  public constructor(public readonly url: string) {
    ReplayEventSource.instances.push(this);
  }

  public addEventListener(type: string, listener: EventListener): void {
    const listeners = this.listeners.get(type) ?? [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  public close(): void {}

  public emit(type: string, data: unknown): void {
    if (type === "snapshot") {
      const state = useFactoryTimelineStore.getState();
      const tracesByWorkID = state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {};
      seedTimelineSnapshot(data as DashboardSnapshot, tracesByWorkID);
    }

    const event = new MessageEvent(type, {
      data: JSON.stringify(data),
    });

    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }

  public emitOpen(): void {
    this.onopen?.(new Event("open"));
  }
}

function seedTimelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: WorldState["tracesByWorkID"] = {},
): void {
  useFactoryTimelineStore.setState({
    events: [],
    latestTick: snapshot.tick_count,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: snapshot.tick_count,
    worldViewCache: {
      [snapshot.tick_count]: timelineSnapshotFromDashboardSnapshot(snapshot, {
        tracesByWorkID,
      }),
    },
  });
}

function timelineSnapshotFromDashboardSnapshot(
  snapshot: DashboardSnapshot,
  overrides: Partial<
    Pick<
      WorldState,
      "relationsByWorkID" | "tracesByWorkID" | "workstationRequestsByDispatchID" | "workRequestsByID"
    >
  > = {},
): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: overrides.relationsByWorkID ?? {},
    tracesByWorkID: overrides.tracesByWorkID ?? {},
    workstationRequestsByDispatchID: overrides.workstationRequestsByDispatchID ?? {},
    workRequestsByID: overrides.workRequestsByID ?? {},
  };
}

export interface ReplayHarness {
  emitMessage: (message: unknown) => void;
  emitSnapshot: (snapshot: DashboardSnapshot) => void;
  getLastStream: () => ReplayEventSource;
  getStreams: () => ReplayEventSource[];
  install: () => void;
  replayEvents: (events: FactoryEvent[]) => Promise<void>;
  replayFixture: (fixtureID: ReplayFixtureID) => Promise<FactoryEvent[]>;
  reset: () => void;
}

export function createReplayHarness(): ReplayHarness {
  function getStreams(): ReplayEventSource[] {
    return [...ReplayEventSource.instances];
  }

  function getLastStream(): ReplayEventSource {
    const stream = ReplayEventSource.instances.at(-1);
    if (!stream) {
      throw new Error("expected factory event stream to be opened");
    }

    return stream;
  }

  function install(): void {
    ReplayEventSource.instances = [];
    originalEventSource = globalThis.EventSource;
    globalThis.EventSource = ReplayEventSource as unknown as typeof EventSource;
  }

  function reset(): void {
    ReplayEventSource.instances = [];
    if (typeof originalEventSource === "undefined") {
      delete (globalThis as { EventSource?: typeof EventSource }).EventSource;
      return;
    }

    globalThis.EventSource = originalEventSource;
  }

  function emitSnapshot(snapshot: DashboardSnapshot): void {
    useFactoryTimelineStore.setState({
      events: [],
      latestTick: snapshot.tick_count,
      mode: "current",
      receivedEventIDs: [],
      selectedTick: snapshot.tick_count,
      worldViewCache: {
        [snapshot.tick_count]: timelineSnapshotFromDashboardSnapshot(snapshot),
      },
    });

    getLastStream().emit("message", {
      context: {
        eventTime: "2026-04-22T17:00:00Z",
        sequence: snapshot.tick_count,
        tick: snapshot.tick_count,
      },
      id: `replay-harness/snapshot/${snapshot.tick_count}`,
      payload: {},
      type: "RUN_RESPONSE",
    } satisfies FactoryEvent);
  }

  function emitMessage(message: unknown): void {
    getLastStream().emit("message", message);
  }

  async function replayEvents(events: FactoryEvent[]): Promise<void> {
    const targetTick = events.reduce(
      (latestTick, event) => Math.max(latestTick, event.context.tick),
      0,
    );
    const stream = getLastStream();

    await act(async () => {
      stream.emitOpen();
      for (const event of events) {
        stream.emit("message", event);
      }
    });

    await waitFor(() => {
      if (useFactoryTimelineStore.getState().selectedTick < targetTick) {
        throw new Error(`expected replay to reach tick ${targetTick}`);
      }
    }, { timeout: REPLAY_SETTLE_TIMEOUT_MS });
  }

  async function replayFixture(fixtureID: ReplayFixtureID): Promise<FactoryEvent[]> {
    const events = loadReplayFixtureEvents(fixtureID);
    await replayEvents(events);
    return events;
  }

  return {
    emitMessage,
    emitSnapshot,
    getLastStream,
    getStreams,
    install,
    replayEvents,
    replayFixture,
    reset,
  };
}

