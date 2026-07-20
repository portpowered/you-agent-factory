import type {
  FactoryDefinition,
  FactoryEvent,
  FactoryEventType,
} from "@you-agent-factory/client";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import type { FactoryReplayWorldReducer } from "@you-agent-factory/factory-replay";
import { describe, expect, it, vi } from "vitest";

import {
  createFactoryEmulatorInstance,
  selectFactoryEmulatorError,
  selectFactoryEmulatorEvents,
  selectFactoryEmulatorReplay,
} from "./factory-emulator-instance";

interface EvidenceState {
  appliedEventIDs: string[];
  selectedTick: number;
}

const reducer: FactoryReplayWorldReducer<EvidenceState, EvidenceState> = {
  applyEvent: (state, event) => ({
    ...state,
    appliedEventIDs: [...state.appliedEventIDs, event.id],
  }),
  createState: (selectedTick) => ({ appliedEventIDs: [], selectedTick }),
  projectWorld: (state) => structuredClone(state),
};

const factory = {
  name: "zustand-adapter-factory",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      name: "task",
      states: [{ name: "ready", type: "INITIAL" }],
    },
  ],
} satisfies FactoryDefinition;

const scenario = {
  schemaVersion: "factory-emulator-scenario/v1",
  id: "zustand-adapter-scenario",
  factory: { name: factory.name },
  seed: "zustand-adapter-seed",
  startAt: "2026-07-19T00:00:00.000Z",
  rules: [],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

function createInstance(
  beforeCommit?: (events: readonly FactoryEvent[]) => Promise<void>,
) {
  return createFactoryEmulatorInstance({
    beforeCommit,
    cloneState: structuredClone,
    factory,
    reducer,
    scenario,
  });
}

function event(
  id: string,
  tick: number,
  sequence: number,
  type: FactoryEventType = "WORK_REQUEST",
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type,
    context: {
      sequence,
      tick,
      eventTime: `2026-07-19T00:00:${String(sequence).padStart(2, "0")}.000Z`,
    },
    payload: { type: "FACTORY_REQUEST_BATCH" },
  } as FactoryEvent;
}

function deferred() {
  let resolvePromise: (() => void) | undefined;
  const promise = new Promise<void>((resolve) => {
    resolvePromise = resolve;
  });
  return { promise, resolve: () => resolvePromise?.() };
}

describe("createFactoryEmulatorInstance", () => {
  it("creates independent stores, sinks, histories, errors, and commands", async () => {
    const first = createInstance();
    const second = createInstance();

    expect(first.store).not.toBe(second.store);
    expect(first.sink).not.toBe(second.sink);
    expect(first.commands).not.toBe(second.commands);

    await first.sink.write([event("first-only", 4, 1)]);

    expect(selectFactoryEmulatorEvents(first.store.getState())).toHaveLength(1);
    expect(selectFactoryEmulatorEvents(second.store.getState())).toEqual([]);
    expect(first.store.getState().selectedTick).toBe(4);
    expect(second.store.getState().selectedTick).toBe(0);
    expect(first.store.getState().sessionStatus.phase).toBe("idle");
    expect(second.store.getState().sessionStatus.phase).toBe("idle");
    expect(selectFactoryEmulatorError(second.store.getState())).toBeUndefined();
  });

  it("commits an accepted batch canonically and refreshes a same-tick projection", async () => {
    const instance = createInstance();
    const firstProjection = selectFactoryEmulatorReplay(
      instance.store.getState(),
    );

    await instance.sink.write([event("later", 2, 3), event("earlier", 1, 1)]);
    const beforeSameTick = selectFactoryEmulatorReplay(
      instance.store.getState(),
    );
    await instance.sink.write([event("same-tick", 2, 2), event("later", 2, 3)]);
    const state = instance.store.getState();

    expect(firstProjection).not.toBe(beforeSameTick);
    expect(state.events.map(({ id }) => id)).toEqual([
      "earlier",
      "same-tick",
      "later",
    ]);
    expect(state.latestTick).toBe(2);
    expect(state.selectedTick).toBe(2);
    expect(state.replay).not.toBe(beforeSameTick);
    expect(state.replay.world.appliedEventIDs).toEqual([
      "earlier",
      "same-tick",
      "later",
    ]);
    expect(state.replay.checkpoint.acceptedEventIDs).toEqual([
      "earlier",
      "same-tick",
      "later",
    ]);
  });

  it("serializes concurrent sink writes without losing an accepted batch", async () => {
    const firstWrite = deferred();
    const observed: string[] = [];
    const instance = createInstance(async (events) => {
      const id = events[0]?.id ?? "missing";
      observed.push(id);
      if (id === "first") await firstWrite.promise;
    });

    const first = instance.sink.write([event("first", 1, 1)]);
    const second = instance.sink.write([event("second", 2, 2)]);
    await vi.waitFor(() => expect(observed).toEqual(["first"]));
    firstWrite.resolve();
    await Promise.all([first, second]);

    expect(observed).toEqual(["first", "second"]);
    expect(instance.store.getState().events.map(({ id }) => id)).toEqual([
      "first",
      "second",
    ]);
  });

  it("preserves committed replay after rejection and retries through the session contract", async () => {
    let reject = false;
    const instance = createInstance(async () => {
      if (reject) throw new Error("temporary recording failure");
    });
    const sibling = createInstance();
    await instance.sink.write([event("already-committed", 1, 1)]);
    const initialReplay = structuredClone(instance.store.getState().replay);
    reject = true;

    await expect(instance.commands.start()).rejects.toThrow(
      "temporary recording failure",
    );

    const rejected = instance.store.getState();
    expect(rejected.events.map(({ id }) => id)).toEqual(["already-committed"]);
    expect(rejected.replay).toEqual(initialReplay);
    expect(rejected.sessionStatus).toMatchObject({
      phase: "error",
      pendingTransaction: { command: "start", phase: "sink-write" },
    });
    expect(rejected.error).toEqual({
      command: "start",
      kind: "event-sink-rejected",
      message: "temporary recording failure",
      recoverable: true,
    });
    expect(sibling.store.getState().error).toBeUndefined();
    expect(sibling.store.getState().events).toEqual([]);

    reject = false;
    await instance.commands.retry();

    const retried = instance.store.getState();
    expect(retried.events).toHaveLength(4);
    expect(retried.error).toBeUndefined();
    expect(retried.sessionStatus.phase).toBe("idle");
  });
});
