import type {
  FactoryDefinition,
  FactoryEvent,
  FactoryEventType,
} from "@you-agent-factory/client";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import type { FactoryReplayWorldReducer } from "@you-agent-factory/factory-replay";
import { describe, expect, it } from "vitest";

import {
  createFactoryEmulatorInstance,
  selectFactoryEmulatorError,
} from "./factory-emulator-instance";
import { selectFactoryEmulatorSubmission } from "./factory-emulator-submission";

const factory = {
  name: "submission-adapter-factory",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      handlingBehavior: ["DEFAULT"],
      name: "task",
      states: [{ name: "ready", type: "INITIAL" }],
    },
  ],
} satisfies FactoryDefinition;

const scenario = {
  schemaVersion: "factory-emulator-scenario/v1",
  id: "submission-adapter-scenario",
  factory: { name: factory.name },
  seed: "submission-adapter-seed",
  startAt: "2026-07-19T00:00:00.000Z",
  rules: [],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

const reducer: FactoryReplayWorldReducer<string[], string[]> = {
  applyEvent: (state, acceptedEvent) => [...state, acceptedEvent.id],
  createState: () => [],
  projectWorld: structuredClone,
};

function createInstance(
  beforeCommit?: (events: readonly FactoryEvent[]) => Promise<void>,
  factoryDefinition: FactoryDefinition = factory,
) {
  return createFactoryEmulatorInstance({
    beforeCommit,
    cloneState: structuredClone,
    factory: factoryDefinition,
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
      eventTime: `2026-07-19T00:00:${sequence}.000Z`,
      sequence,
      tick,
    },
    payload: { type: "FACTORY_REQUEST_BATCH" },
  } as FactoryEvent;
}

describe("factory emulator text submission", () => {
  it("submits one text item with deterministic identity and clears the accepted draft", async () => {
    const first = createInstance();
    const second = createInstance();
    await Promise.all([first.commands.start(), second.commands.start()]);
    first.commands.setDraft("Interactive task");
    second.commands.setDraft("Interactive task");

    const submission = {
      content: [{ text: "Interactive task", type: "text" }],
      workTypeName: "task",
    } as const;
    await first.commands.submit(submission);
    await second.commands.submit(submission);

    expect(first.store.getState().submission).toEqual({
      draft: "",
      nextOrdinal: 2,
      status: "idle",
    });
    const request = first.store
      .getState()
      .events.find(({ type }) => type === "WORK_REQUEST");
    const siblingRequest = second.store
      .getState()
      .events.find(({ type }) => type === "WORK_REQUEST");
    expect(request?.payload).toMatchObject({
      source: "emulator",
      works: [
        {
          name: "website-interactive-work-1",
          payload: "Interactive task",
          workTypeName: "task",
        },
      ],
    });
    expect(siblingRequest).toEqual(request);
  });

  it("preserves a failed draft and clears it after the pending retry succeeds", async () => {
    let rejectSubmission = false;
    const instance = createInstance(async (events) => {
      if (
        rejectSubmission &&
        events.some(({ type }) => type === "WORK_REQUEST")
      ) {
        throw new Error("temporary interactive submission failure");
      }
    });
    await instance.commands.start();
    instance.commands.setDraft("Keep this task");
    rejectSubmission = true;

    await expect(
      instance.commands.submit({
        content: [{ text: "Keep this task", type: "text" }],
        workTypeName: "task",
      }),
    ).rejects.toThrow("temporary interactive submission failure");
    expect(instance.store.getState().submission.draft).toBe("Keep this task");
    expect(selectFactoryEmulatorError(instance.store.getState())).toMatchObject(
      {
        command: "submit",
        kind: "submission-rejected",
        recoverable: true,
      },
    );
    expect(
      instance.store
        .getState()
        .events.some(({ type }) => type === "WORK_REQUEST"),
    ).toBe(false);

    rejectSubmission = false;
    await instance.commands.retry();
    expect(instance.store.getState().submission).toEqual({
      draft: "",
      nextOrdinal: 2,
      status: "idle",
    });
  });

  it("disables submission outside the current valid run or unique default target", async () => {
    const instance = createInstance();
    const submission = {
      content: [{ text: "A task", type: "text" }],
      workTypeName: "task",
    } as const;

    await expect(instance.commands.submit(submission)).resolves.toMatchObject({
      reason: "Start the emulator before submitting.",
      status: "disabled",
    });
    await instance.commands.start();
    await instance.sink.write([event("future", 3, 20)]);
    instance.commands.selectTick(0);
    expect(
      selectFactoryEmulatorSubmission(instance.store.getState(), factory),
    ).toMatchObject({ factoryState: "active", isCurrent: false });
    await expect(instance.commands.submit(submission)).resolves.toMatchObject({
      reason: "Return to the current tick before submitting.",
      status: "disabled",
    });

    const noDefault = createInstance(undefined, {
      ...factory,
      workTypes: factory.workTypes.map((workType) => ({
        ...workType,
        handlingBehavior: [],
      })),
    });
    await noDefault.commands.start();
    await expect(noDefault.commands.submit(submission)).resolves.toMatchObject({
      reason: "No eligible default Work Type is available.",
      status: "disabled",
    });
  });
});
