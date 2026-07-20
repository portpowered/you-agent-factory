import type { FactoryDefinition } from "@you-agent-factory/client";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import type { FactoryReplayWorldReducer } from "@you-agent-factory/factory-replay";
import { useEffect, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import { createFactoryEmulatorInstance } from "../state/factory-emulator-instance";
import { FactoryEmulatorSubmission } from "./factory-emulator-submission";

const factory = {
  name: "submission-story-factory",
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
  id: "submission-story-scenario",
  factory: { name: factory.name },
  seed: "submission-story-seed",
  startAt: "2026-07-19T00:00:00.000Z",
  rules: [],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

const reducer: FactoryReplayWorldReducer<string[], string[]> = {
  applyEvent: (state, event) => [...state, event.id],
  createState: () => [],
  projectWorld: (state) => structuredClone(state),
};

function SubmissionStory({ reject = false }: { reject?: boolean }) {
  const [instance] = useState(() =>
    createFactoryEmulatorInstance({
      beforeCommit: async (events) => {
        if (reject && events.some(({ type }) => type === "WORK_REQUEST")) {
          throw new Error("The emulator could not accept this work.");
        }
      },
      cloneState: structuredClone,
      factory,
      reducer,
      scenario,
    }),
  );

  useEffect(() => {
    void instance.commands.start();
  }, [instance]);

  return (
    <div className="w-full max-w-2xl p-4">
      <FactoryEmulatorSubmission factory={factory} instance={instance} />
    </div>
  );
}

async function enterDraft(canvasElement: HTMLElement, draft: string) {
  const canvas = within(canvasElement);
  const textbox = await canvas.findByRole("textbox", { name: "Submit text" });
  await waitFor(() => expect(textbox).toBeEnabled());
  await userEvent.clear(textbox);
  await userEvent.type(textbox, draft);
  return { canvas, textbox };
}

export default {
  title: "Agent Factory/Emulator/Submission",
  component: FactoryEmulatorSubmission,
};

export const Accepted = {
  render: () => <SubmissionStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const { textbox } = await enterDraft(canvasElement, "Browser task");
    await userEvent.type(textbox, "{enter}");
    await waitFor(() => expect(textbox).toHaveValue(""));
  },
};

export const FailurePreservesDraft = {
  render: () => <SubmissionStory reject />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const { canvas, textbox } = await enterDraft(canvasElement, "Keep me");
    await userEvent.click(canvas.getByRole("button", { name: "Submit" }));
    await expect(canvas.findByRole("alert")).resolves.toHaveTextContent(
      "The emulator could not accept this work.",
    );
    expect(textbox).toHaveValue("Keep me");
  },
};

export const AcceptedMobile = {
  ...Accepted,
  parameters: { viewport: { defaultViewport: "mobile1" } },
};
