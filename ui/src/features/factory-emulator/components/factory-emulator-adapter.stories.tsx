import type {
  FactoryDefinition,
  FactoryEvent,
  FactoryEventType,
} from "@you-agent-factory/client";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import type { FactoryReplayWorldReducer } from "@you-agent-factory/factory-replay";
import { FactoryEmulatorControls } from "@you-agent-factory/factory-visualizers";
import { useEffect, useRef, useState } from "react";
import { useStore } from "zustand";

import { Button } from "../../../components/ui";
import {
  createFactoryEmulatorInstance,
  type FactoryEmulatorInstance,
} from "../state/factory-emulator-instance";
import {
  selectFactoryEmulatorControls,
  selectFactoryEmulatorTimeline,
} from "../state/factory-emulator-presentation";
import { FactoryEmulatorSubmission } from "./factory-emulator-submission";

interface EvidenceState {
  readonly eventIDs: readonly string[];
}

const factory = {
  name: "adapter-browser-story-factory",
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
  id: "adapter-browser-story-scenario",
  factory: { name: factory.name },
  seed: "adapter-browser-story-seed",
  startAt: "2026-07-19T00:00:00.000Z",
  rules: [],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

const reducer: FactoryReplayWorldReducer<EvidenceState, EvidenceState> = {
  applyEvent: (state, event) => ({
    eventIDs: [...state.eventIDs, event.id],
  }),
  createState: () => ({ eventIDs: [] }),
  projectWorld: (state) => structuredClone(state),
};

const timelineMessages = {
  alreadyFollowingLatest: "Following the latest tick.",
  currentMode: "Showing the current Factory.",
  disabled: "Timeline selection is disabled by the host.",
  followLatest: "Follow latest",
  historyMode: "Viewing Factory history.",
  position: (selected: string, latest: string) =>
    `Tick ${selected} of ${latest}`,
  regionLabel: "Factory replay timeline",
  sliderLabel: "Select replay tick",
  title: "Replay timeline",
  unavailable: "No replay ticks are available.",
};

function evidenceEvent(
  id: string,
  tick: number,
  sequence: number,
  type: FactoryEventType = "FACTORY_STATE_CHANGED",
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type,
    context: {
      eventTime: `2026-07-19T00:00:${String(sequence).padStart(2, "0")}.000Z`,
      sequence,
      tick,
    },
    payload: { type: "FACTORY_STATE_CHANGED" },
  } as FactoryEvent;
}

function createInstance(rejectSubmissions: boolean) {
  return createFactoryEmulatorInstance({
    beforeCommit: async (events) => {
      if (
        rejectSubmissions &&
        events.some(({ type }) => type === "WORK_REQUEST")
      ) {
        throw new Error("The emulator could not accept this work.");
      }
    },
    cloneState: structuredClone,
    factory,
    reducer,
    scenario,
  });
}

function runtimeStatus(
  instance: FactoryEmulatorInstance<EvidenceState, EvidenceState>,
) {
  const state = instance.store.getState();
  if (state.error) return { label: "Error", tone: "danger" } as const;
  if (state.mode === "history")
    return { label: "Viewing history", tone: "warning" } as const;
  if (state.playback.status === "playing")
    return { label: "Playing", tone: "success" } as const;
  return { label: "Paused", tone: "neutral" } as const;
}

function AdapterStory({ rejectSubmissions = false }) {
  const [instance] = useState(() => createInstance(rejectSubmissions));
  const nextEvidenceTick = useRef(3);
  const state = useStore(instance.store);
  const controls = selectFactoryEmulatorControls(state);
  const timeline = selectFactoryEmulatorTimeline(state);

  useEffect(() => {
    void (async () => {
      await instance.commands.start();
      await instance.sink.write([
        evidenceEvent("browser-evidence-1", 1, 1),
        evidenceEvent("browser-evidence-2", 2, 2),
      ]);
    })();
  }, [instance]);

  const acceptNextEvent = async () => {
    const tick = nextEvidenceTick.current;
    nextEvidenceTick.current += 1;
    await instance.sink.write([
      evidenceEvent(`browser-evidence-${tick}`, tick, tick),
    ]);
  };

  return (
    <main
      aria-label="Factory emulator adapter browser evidence"
      className="mx-auto grid w-full min-w-0 max-w-4xl gap-4 p-4"
    >
      <FactoryEmulatorControls
        {...controls}
        formatTick={String}
        onFollowLatest={instance.commands.followCurrent}
        onPause={instance.commands.pause}
        onPlay={instance.commands.play}
        onRestart={() => void instance.commands.restart()}
        onSelectTick={instance.commands.selectTick}
        onSpeedChange={instance.commands.setSpeed}
        onStep={() => void instance.commands.step()}
        runtimeStatus={runtimeStatus(instance)}
        timeline={{ messages: timelineMessages, state: timeline }}
      />
      <div className="flex min-w-0 flex-wrap gap-2">
        <Button onClick={() => void acceptNextEvent()} tone="outline">
          Accept next event
        </Button>
      </div>
      <FactoryEmulatorSubmission factory={factory} instance={instance} />
    </main>
  );
}

export default {
  title: "Agent Factory/Emulator/Adapter Demo",
  component: AdapterStory,
  parameters: { layout: "fullscreen" },
};

export const Interactive = {
  render: () => <AdapterStory />,
};

export const SubmissionFailure = {
  render: () => <AdapterStory rejectSubmissions />,
};
