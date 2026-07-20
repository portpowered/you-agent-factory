import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import type { FactoryReplayWorldReducer } from "@you-agent-factory/factory-replay";
import { describe, expect, it } from "vitest";

import { createFactoryEmulatorInstance } from "../state/factory-emulator-instance";
import { FactoryEmulatorSubmission } from "./factory-emulator-submission";

const factory = {
  name: "submission-component-factory",
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
  id: "submission-component-scenario",
  factory: { name: factory.name },
  seed: "submission-component-seed",
  startAt: "2026-07-19T00:00:00.000Z",
  rules: [],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

const reducer: FactoryReplayWorldReducer<string[], string[]> = {
  applyEvent: (state, event) => [...state, event.id],
  createState: () => [],
  projectWorld: structuredClone,
};

function event(): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id: "later-tick",
    type: "WORK_REQUEST",
    context: {
      eventTime: "2026-07-19T00:00:03.000Z",
      sequence: 30,
      tick: 3,
    },
    payload: { type: "FACTORY_REQUEST_BATCH" },
  } as FactoryEvent;
}

describe("FactoryEmulatorSubmission", () => {
  it("connects keyboard submission and history disablement to one instance", async () => {
    const instance = createFactoryEmulatorInstance({
      cloneState: structuredClone,
      factory,
      reducer,
      scenario,
    });
    await instance.commands.start();
    render(<FactoryEmulatorSubmission factory={factory} instance={instance} />);
    const textarea = screen.getByRole("textbox", { name: "Submit text" });

    fireEvent.change(textarea, { target: { value: "Keyboard task" } });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await waitFor(() => expect(textarea).toHaveValue(""));
    expect(
      instance.store
        .getState()
        .events.some(({ type }) => type === "WORK_REQUEST"),
    ).toBe(true);

    await instance.sink.write([event()]);
    instance.commands.selectTick(0);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled(),
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Return to the current tick before submitting.",
    );
  });
});
