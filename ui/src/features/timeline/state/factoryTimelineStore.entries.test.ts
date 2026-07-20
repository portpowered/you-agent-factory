import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import type { StreamDerivedCacheIdentity } from "../lib/stream-derived-cache-identity";
import { useFactoryTimelineStore } from "./factoryTimelineStore";

const eventTime = "2026-07-13T12:00:00.000Z";

function streamIdentity(
  factorySessionID: string,
  streamGenerationID: string,
): StreamDerivedCacheIdentity {
  return {
    backendScopeID: "backend-a",
    factorySessionID,
    logicalSessionKeyID: "shared-logical-alias",
    streamGenerationID,
  };
}

function event(id: string, tick: number): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
    },
    id,
    payload: {
      factory: {
        workers: [],
        workTypes: [],
        workstations: [],
      },
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  };
}

afterEach(() => {
  useFactoryTimelineStore.getState().reset();
});

describe("factory timeline stream entries", () => {
  it("isolates exact sessions even when their logical alias matches", () => {
    const sessionA = streamIdentity("session-a-uuid", "generation-1");
    const sessionB = streamIdentity("session-b-uuid", "generation-1");
    const store = useFactoryTimelineStore.getState();

    store.appendEventForEntry(sessionA, event("event-a", 1));
    store.appendEventForEntry(sessionB, event("event-b", 4));
    store.selectTickForEntry(sessionA, 1);
    store.activateEntry(sessionB);

    const entryA = useFactoryTimelineStore
      .getState()
      .entryForIdentity(sessionA);
    const entryB = useFactoryTimelineStore
      .getState()
      .entryForIdentity(sessionB);
    expect(entryA).toMatchObject({
      latestTick: 1,
      mode: "fixed",
      receivedEventIDs: ["event-a"],
      selectedTick: 1,
    });
    expect(entryB).toMatchObject({
      latestTick: 4,
      mode: "current",
      receivedEventIDs: ["event-b"],
      selectedTick: 4,
    });
    expect(entryA?.events.map(({ id }) => id)).toEqual(["event-a"]);
    expect(entryB?.events.map(({ id }) => id)).toEqual(["event-b"]);
    expect(entryA?.currentReplayCheckpoint?.afterEventId).toBe("event-a");
    expect(entryB?.currentReplayCheckpoint?.afterEventId).toBe("event-b");
    expect(entryA?.worldViewCache[4]).toBeUndefined();
    expect(entryB?.worldViewCache[1]).toBeUndefined();
    expect(entryA?.materializedWorkOutcomeState.cursor?.eventID).toBe(
      "event-a",
    );
    expect(entryB?.materializedWorkOutcomeState.cursor?.eventID).toBe(
      "event-b",
    );
    expect(entryA?.materializedWorkOutcomeState).not.toBe(
      entryB?.materializedWorkOutcomeState,
    );

    const active = useFactoryTimelineStore.getState();
    expect(active.activeEntryKey).not.toBeNull();
    expect(active.events.map(({ id }) => id)).toEqual(["event-b"]);
    expect(active.latestTick).toBe(4);
  });

  it("keeps generations of one resolved session in separate accumulators", () => {
    const generationA = streamIdentity("session-a-uuid", "generation-1");
    const generationB = streamIdentity("session-a-uuid", "generation-2");
    const store = useFactoryTimelineStore.getState();

    store.appendEventForEntry(generationA, event("old-generation", 3));
    store.activateEntry(generationB);

    const oldEntry = useFactoryTimelineStore
      .getState()
      .entryForIdentity(generationA);
    const replacementEntry = useFactoryTimelineStore
      .getState()
      .entryForIdentity(generationB);
    expect(oldEntry?.events.map(({ id }) => id)).toEqual(["old-generation"]);
    expect(oldEntry?.currentReplayCheckpoint?.afterEventId).toBe(
      "old-generation",
    );
    expect(replacementEntry).toMatchObject({
      currentReplayCheckpoint: undefined,
      events: [],
      latestTick: 0,
      mode: "current",
      receivedEventIDs: [],
      selectedTick: 0,
    });
    expect(replacementEntry?.materializedWorkOutcomeState).toMatchObject({
      counts: {
        completed: 0,
        dispatched: 0,
        failed: 0,
        inFlight: 0,
        queued: 0,
      },
      cursor: null,
      version: 1,
    });
    expect(replacementEntry?.materializedWorkOutcomeState).not.toBe(
      oldEntry?.materializedWorkOutcomeState,
    );

    store.appendEventForEntry(generationA, event("late-old-generation", 5));
    const active = useFactoryTimelineStore.getState();
    expect(active.events).toEqual([]);
    expect(active.currentReplayCheckpoint).toBeUndefined();
    expect(active.materializedWorkOutcomeState.cursor).toBeNull();
    expect(
      active.entryForIdentity(generationA)?.events.map(({ id }) => id),
    ).toEqual(["old-generation", "late-old-generation"]);
  });
});

describe("factory timeline entry binding", () => {
  it("rejects unresolved identities before creating an entry", () => {
    const unresolved = streamIdentity("~default", "generation-1");

    expect(() =>
      useFactoryTimelineStore
        .getState()
        .appendEventForEntry(unresolved, event("unresolved", 1)),
    ).toThrow(/resolved Factory Session UUID/);
    expect(useFactoryTimelineStore.getState().entriesByKey).toEqual({});
  });

  it("binds a preflight identity to an unowned compatibility seed", () => {
    const identity = streamIdentity("session-a-uuid", "generation-1");
    const seededEvent = event("seeded-before-preflight", 7);
    useFactoryTimelineStore.setState({
      events: [seededEvent],
      latestTick: 7,
      receivedEventIDs: [seededEvent.id],
      selectedTick: 7,
    });

    useFactoryTimelineStore.getState().activateEntry(identity);

    const entry = useFactoryTimelineStore.getState().entryForIdentity(identity);
    expect(entry?.events.map(({ id }) => id)).toEqual([
      "seeded-before-preflight",
    ]);
    expect(entry?.latestTick).toBe(7);
    expect(entry?.selectedTick).toBe(7);
  });
});

describe("factory timeline session cleanup", () => {
  it("removes every generation of one session without changing another session", () => {
    const sessionA = streamIdentity("session-a-uuid", "generation-1");
    const nextSessionAGeneration = streamIdentity(
      "session-a-uuid",
      "generation-2",
    );
    const sessionB = streamIdentity("session-b-uuid", "generation-1");
    const store = useFactoryTimelineStore.getState();

    store.appendEventForEntry(sessionA, event("event-a-1", 1));
    store.appendEventForEntry(nextSessionAGeneration, event("event-a-2", 2));
    store.appendEventForEntry(sessionB, event("event-b", 4));
    store.activateEntry(sessionA);
    store.removeEntriesForSession(sessionA.factorySessionID);

    const state = useFactoryTimelineStore.getState();
    expect(state.entryForIdentity(sessionA)).toBeUndefined();
    expect(state.entryForIdentity(nextSessionAGeneration)).toBeUndefined();
    expect(
      state.entryForIdentity(sessionB)?.events.map(({ id }) => id),
    ).toEqual(["event-b"]);
    expect(state.activeEntryKey).toBeNull();
    expect(state.events).toEqual([]);
  });
});
