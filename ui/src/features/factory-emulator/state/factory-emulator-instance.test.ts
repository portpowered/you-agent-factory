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
import {
  selectFactoryEmulatorControls,
  selectFactoryEmulatorTimeline,
} from "./factory-emulator-presentation";

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
});

describe("factory emulator replay selection", () => {
  it("keeps historical inspection fixed while accepted events advance the head", async () => {
    const instance = createInstance();
    await instance.sink.write([
      event("tick-one", 1, 1),
      event("tick-four", 4, 2),
    ]);

    expect(instance.commands.selectTick(1)).toEqual({ status: "accepted" });
    expect(instance.store.getState()).toMatchObject({
      latestTick: 4,
      mode: "history",
      playback: { status: "paused" },
      selectedTick: 1,
    });
    expect(instance.store.getState().replay.world.appliedEventIDs).toEqual([
      "tick-one",
    ]);

    await instance.sink.write([event("tick-nine", 9, 3)]);

    const historical = instance.store.getState();
    expect(historical.latestTick).toBe(9);
    expect(historical.selectedTick).toBe(1);
    expect(historical.mode).toBe("history");
    expect(historical.replay.world.appliedEventIDs).toEqual(["tick-one"]);
    expect(selectFactoryEmulatorTimeline(historical)).toEqual({
      earliestTick: 0,
      latestTick: 9,
      mode: "history",
      selectedTick: 1,
      status: "available",
    });
  });

  it("follows the latest projection again without synthesizing sparse ticks", async () => {
    const instance = createInstance();
    await instance.sink.write([
      event("tick-two", 2, 1),
      event("tick-eight", 8, 2),
    ]);
    instance.commands.selectTick(5);

    expect(instance.store.getState().replay.world.appliedEventIDs).toEqual([
      "tick-two",
    ]);
    expect(instance.commands.followCurrent()).toEqual({ status: "accepted" });
    expect(instance.store.getState()).toMatchObject({
      latestTick: 8,
      mode: "current",
      selectedTick: 8,
    });
    expect(instance.store.getState().replay.world.appliedEventIDs).toEqual([
      "tick-two",
      "tick-eight",
    ]);

    await instance.sink.write([event("tick-twelve", 12, 3)]);
    expect(instance.store.getState()).toMatchObject({
      latestTick: 12,
      mode: "current",
      selectedTick: 12,
    });
  });
});

describe("factory emulator playback commands", () => {
  it("exposes controlled playback outcomes without scheduling host time", async () => {
    vi.useFakeTimers();
    try {
      const instance = createInstance();

      await expect(instance.commands.step()).resolves.toEqual({
        command: "step",
        reason: "Start the emulator before stepping.",
        status: "disabled",
      });
      expect(instance.commands.play()).toEqual({ status: "accepted" });
      expect(instance.commands.setSpeed(4)).toEqual({ status: "accepted" });
      expect(selectFactoryEmulatorControls(instance.store.getState())).toEqual({
        disabledActions: ["play"],
        isPlaying: true,
        speed: 4,
      });
      expect(vi.getTimerCount()).toBe(0);

      expect(instance.commands.pause()).toEqual({ status: "accepted" });
      expect(instance.store.getState().playback.status).toBe("paused");
      expect(instance.commands.selectTick(-1)).toEqual({
        command: "selectTick",
        reason: "Select a logical tick from 0 through 0.",
        status: "disabled",
      });
      await expect(instance.commands.retry()).resolves.toEqual({
        command: "retry",
        reason: "There is no failed command to retry.",
        status: "disabled",
      });

      await instance.commands.start();
      await expect(instance.commands.step()).resolves.toEqual({
        status: "accepted",
      });
      expect(vi.getTimerCount()).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps host-wrapped execution controls available in history", async () => {
    const instance = createInstance();
    await instance.sink.write([
      event("tick-one", 1, 1),
      event("tick-three", 3, 2),
    ]);
    instance.commands.selectTick(1);

    expect(selectFactoryEmulatorControls(instance.store.getState())).toEqual({
      disabledActions: ["pause"],
      isPlaying: false,
      speed: 1,
    });
    expect(instance.commands.play()).toMatchObject({
      command: "play",
      status: "disabled",
    });
    await expect(instance.commands.step()).resolves.toMatchObject({
      command: "step",
      status: "disabled",
    });

    instance.commands.followCurrent();
    expect(instance.commands.play()).toEqual({ status: "accepted" });
  });
});

describe("factory emulator event sink failures", () => {
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
