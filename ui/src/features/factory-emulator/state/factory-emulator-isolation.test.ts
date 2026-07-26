import type {
  FactoryDefinition,
  FactoryEvent,
  FactoryEventType,
} from "@you-agent-factory/client";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import type { FactoryReplayWorldReducer } from "@you-agent-factory/factory-replay";
import { afterEach, describe, expect, it } from "vitest";

import { useFactoryTimelineStore } from "../../timeline/public/store";
import { createFactoryEmulatorInstance } from "../public";

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
  name: "zustand-adapter-executable-factory",
  orchestrator: { kind: "PETRI" },
  workers: [{ name: "scripted-worker", type: "AGENT_WORKER" }],
  workstations: [
    {
      inputs: [{ state: "ready", workType: "task" }],
      name: "scripted-run",
      outputs: [{ state: "done", workType: "task" }],
      type: "AGENT_RUN",
      worker: "scripted-worker",
    },
  ],
  workTypes: [
    {
      name: "task",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
} satisfies FactoryDefinition;

const scenario = {
  schemaVersion: "factory-emulator-scenario/v1",
  id: "zustand-adapter-seeded-scenario",
  factory: { name: factory.name },
  seed: "zustand-adapter-seed",
  startAt: "2026-07-19T00:00:00.000Z",
  initialSubmissions: [
    {
      input: "seeded input",
      name: "seeded-work",
      state: "ready",
      workType: "task",
    },
  ],
  rules: [
    {
      cursor: { input: "rootWorkId", scope: "lineage" },
      exhaustion: "repeat-last",
      id: "scripted-outcome",
      outcomes: [{ durationMs: 25, result: "accepted" }],
      selector: { workstation: "scripted-run" },
    },
  ],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

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

afterEach(() => {
  useFactoryTimelineStore.getState().reset();
});

describe("factory emulator restart isolation", () => {
  it("rebuilds one seeded run without changing a sibling or hosted timeline", async () => {
    let rejectNextTargetWrite = false;
    const target = createInstance(async () => {
      if (rejectNextTargetWrite) throw new Error("target presentation error");
    });
    const sibling = createInstance();
    const hosted = useFactoryTimelineStore.getState();
    hosted.appendEvents([
      event("hosted-one", 1, 1),
      event("hosted-five", 5, 2),
    ]);
    hosted.selectTick(1);

    await target.commands.start();
    await sibling.commands.start();
    const seededHistory = structuredClone(target.store.getState().events);
    await target.commands.step();
    await target.commands.step();
    const completedHistory = structuredClone(target.store.getState().events);
    const completedTick = target.store.getState().latestTick;

    target.commands.play();
    target.commands.setSpeed(4);
    target.commands.setDraft("interactive input");
    await target.commands.submit({
      content: [{ text: "interactive input", type: "text" }],
      workTypeName: "task",
    });
    await target.sink.write([
      event("target-history", 7, 100),
      event("target-head", 9, 101),
    ]);
    target.commands.selectTick(7);
    rejectNextTargetWrite = true;
    await expect(
      target.sink.write([event("target-rejected", 10, 102)]),
    ).rejects.toThrow("target presentation error");
    rejectNextTargetWrite = false;
    expect(target.store.getState().error).toMatchObject({
      kind: "event-sink-rejected",
      recoverable: true,
    });

    const siblingBeforeRestart = structuredClone(sibling.store.getState());
    const hostedBeforeRestart = {
      events: structuredClone(useFactoryTimelineStore.getState().events),
      latestTick: useFactoryTimelineStore.getState().latestTick,
      mode: useFactoryTimelineStore.getState().mode,
      selectedTick: useFactoryTimelineStore.getState().selectedTick,
    };

    await expect(target.commands.restart()).resolves.toEqual({
      status: "accepted",
    });

    const restarted = target.store.getState();
    expect(restarted.events).toEqual(seededHistory);
    expect(
      restarted.events.some(({ payload }) =>
        JSON.stringify(payload).includes("seeded input"),
      ),
    ).toBe(true);
    expect(restarted.events.some(({ id }) => id === "target-history")).toBe(
      false,
    );
    expect(restarted).toMatchObject({
      commandState: "idle",
      error: undefined,
      latestTick: 0,
      mode: "current",
      playback: { speed: 1, status: "paused" },
      selectedTick: 0,
      sessionLifecycle: "started",
      submission: { draft: "", nextOrdinal: 1, status: "idle" },
    });
    expect(restarted.replay.checkpoint.acceptedEventIDs).toEqual(
      seededHistory.map(({ id }) => id),
    );
    expect(sibling.store.getState()).toEqual(siblingBeforeRestart);
    expect({
      events: useFactoryTimelineStore.getState().events,
      latestTick: useFactoryTimelineStore.getState().latestTick,
      mode: useFactoryTimelineStore.getState().mode,
      selectedTick: useFactoryTimelineStore.getState().selectedTick,
    }).toEqual(hostedBeforeRestart);

    await target.commands.step();
    await target.commands.step();
    expect(target.store.getState().events).toEqual(completedHistory);
    expect(target.store.getState()).toMatchObject({
      latestTick: completedTick,
      mode: "current",
      playback: { status: "paused" },
      selectedTick: completedTick,
      sessionStatus: { phase: "idle" },
    });
  });
});
