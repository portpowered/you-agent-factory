import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import customerSupportRecording from "../examples/support-playback.factory-recording.v1.json";

import {
  FactoryRecordingTopologyReplay,
  type FactoryRecordingTopologyReplayMessages,
} from "./factory-recording-topology-replay";
import { createGermanRecordingMessages } from "./testing/factory-recording-messages";
import { createDenseFactoryRecording } from "./testing/factory-recordings";

const messages: FactoryRecordingTopologyReplayMessages = {
  progress: {
    categories: {
      active: category("active"),
      completed: category("completed"),
      failed: category("failed"),
      queued: category("queued"),
      unclassified: category("unclassified"),
    },
    empty: "No Work has been recorded.",
    regionLabel: "Recorded Work progress",
    title: "Work progress",
    total: (count) => `${count} Work total`,
  },
  regionLabel: "Recorded Factory playback",
  selectedTick: (tick) => `Selected logical tick ${tick}`,
  timeline: {
    alreadyFollowingLatest: "Already following the current recording",
    currentMode: "Following current recording",
    disabled: "Recording playback is disabled",
    followLatest: "Follow latest",
    historyMode: "Inspecting recording history",
    position: (selected, latest) => `Tick ${selected} of ${latest}`,
    regionLabel: "Recording timeline",
    sliderLabel: "Select recording tick",
    title: "Recording timeline",
    unavailable: "Recording timeline unavailable",
  },
  topology: {
    activeDispatches: (count) =>
      `${count} active ${count === 1 ? "Dispatch" : "Dispatches"}`,
    annotationsHidden: "Show annotations",
    annotationsVisible: "Hide annotations",
    empty: "No Factory topology is available at this tick.",
    failed: "The Factory topology could not be shown.",
    inactiveDispatches: "No active Dispatch",
    loading: "Loading Factory topology.",
    nodeLabel: (kind, label) => `${kind}: ${label}`,
    regionLabel: "Recorded Factory topology",
    resourceOccupancy: (occupied, capacity) =>
      `${occupied} of ${capacity} capacity occupied`,
    resourceOccupancyUnavailable: "Occupancy unavailable",
    retry: "Try again",
    selectedNode: "Selected",
    workStateCount: (count) => `${count} Work in this state`,
    workStateCountUnavailable: "Work count unavailable",
  },
  validationFailed: "The Factory recording could not be validated.",
};

const meta = {
  title: "Factory Visualizers/FactoryRecordingTopologyReplay",
  component: FactoryRecordingTopologyReplay,
  args: {
    formatNumber: (value) => new Intl.NumberFormat("en").format(value),
    messages,
    onError: fn(),
    onSelectNode: fn(),
    recording: customerSupportRecording,
  },
  decorators: [
    (Story) => (
      <>
        <Story />
        <button type="button">Sibling example control</button>
      </>
    ),
  ],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof FactoryRecordingTopologyReplay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {
  args: {
    recording: undefined,
    state: { status: "loading" },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("region", { name: messages.topology.regionLabel }),
    ).toHaveAttribute("aria-busy", "true");
    await expect(canvas.getByRole("status")).toHaveTextContent(
      messages.topology.loading,
    );
  },
};

export const EmptyRecording: Story = {
  args: {
    recording: undefined,
    state: { recording: emptyRecording(), status: "ready" },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      within(
        canvas.getByRole("region", { name: messages.topology.regionLabel }),
      ).getByRole("status"),
    ).toHaveTextContent(messages.topology.empty);
    await expect(
      canvas.getByRole("region", { name: messages.progress.regionLabel }),
    ).toHaveAttribute("data-work-progress-total", "0");
  },
};

export const ValidatedRecording: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("region", { name: messages.regionLabel }),
    ).toHaveAttribute("data-selected-tick", "2");
    const workstation = canvas.getByRole("button", {
      name: "workstation: triage",
    });
    await userEvent.click(workstation);
    await expect(args.onSelectNode).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "workstation", label: "triage" }),
    );
  },
};

export const InvalidRecording: Story = {
  args: {
    recording: {
      events: [{ payload: { private: "not reported" } }],
      id: "invalid",
      schemaVersion: "factory-recording/v1",
      title: 42,
    },
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("alert")).toHaveTextContent(
      messages.topology.failed,
    );
    await expect(args.onError).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "recording-validation" }),
    );
    await expect(
      canvas.getByRole("button", { name: "Sibling example control" }),
    ).toBeVisible();
  },
};

export const ProjectionFailure: Story = {
  args: {
    recording: undefined,
    state: {
      error: {
        cause: { code: "INVALID_PROJECTION", name: "ProjectionError" },
        kind: "projection",
        message: "The prepared topology projection could not be read.",
        recoverable: true,
      },
      status: "failed",
    },
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("alert")).toHaveTextContent(
      messages.topology.failed,
    );
    await expect(args.onError).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "projection" }),
    );
    await expect(
      canvas.getByRole("button", { name: "Sibling example control" }),
    ).toBeVisible();
  },
};

export const SameTickHistoryAndCurrent: Story = {
  args: {
    defaultSelectedTick: 1,
    recording: playbackRecording(),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const slider = canvas.getByRole("slider", {
      name: messages.timeline.sliderLabel,
    });
    await expect(canvas.getByText("Tick 1 of 3")).toBeVisible();
    slider.focus();
    await userEvent.keyboard("{ArrowRight}");
    await expect(canvas.getByText("Selected logical tick 3")).toBeVisible();
    await expect(canvas.getByText("1 completed Work")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "workstation: triage" }),
    ).toHaveTextContent("1 active Dispatch");
    await userEvent.click(
      canvas.getByRole("button", { name: messages.timeline.followLatest }),
    );
    await expect(canvas.getByText(messages.timeline.currentMode)).toBeVisible();
  },
};

export const DenseRecording: Story = {
  args: {
    recording: createDenseFactoryRecording(),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const topology = canvas.getByRole("region", {
      name: messages.topology.regionLabel,
    });
    await expect(topology).toHaveAttribute("data-endpoints-valid", "true");
    await expect(
      canvas.getByRole("button", { name: "workstation: Review" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("region", { name: messages.timeline.regionLabel }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("region", { name: messages.progress.regionLabel }),
    ).toHaveAttribute("data-work-progress-total", "2");
  },
};

export const LocalizedRecording: Story = {
  args: {
    formatNumber: new Intl.NumberFormat("de-DE").format,
    messages: createGermanRecordingMessages(),
    recording: createDenseFactoryRecording(),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("region", { name: "Aufgezeichnete Fabrikwiedergabe" }),
    ).toHaveTextContent("Ausgewählter logischer Schritt 7.000");
    await expect(canvas.getByText("2 Aufträge insgesamt")).toBeVisible();
    await expect(
      canvas.getByText("Aktuelle Aufzeichnung verfolgen"),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Arbeitsstation: Review" }),
    ).toBeVisible();
  },
};

export const NarrowViewport: Story = {
  args: {
    recording: createDenseFactoryRecording(),
  },
  parameters: {
    viewport: { defaultViewport: "mobile1" },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const slider = canvas.getByRole("slider", {
      name: messages.timeline.sliderLabel,
    });
    slider.focus();
    await userEvent.keyboard("{ArrowLeft}");
    await expect(slider).toHaveFocus();
    await expect(canvas.getByText(messages.timeline.historyMode)).toBeVisible();
  },
};

function category(name: string) {
  return {
    plural: (count: string) => `${count} ${name} Work`,
    singular: (count: string) => `${count} ${name} Work`,
  };
}

function emptyRecording(): unknown {
  const factory = { name: "empty-local-recording" };
  return {
    events: [
      {
        context: {
          eventTime: "2026-07-18T19:00:00Z",
          sequence: 1,
          sessionId: "storybook-empty-session",
          sessionSequence: 1,
          tick: 0,
        },
        id: "empty-topology",
        payload: { factory },
        schemaVersion: "agent-factory.event.v1",
        type: "INITIAL_STRUCTURE_REQUEST",
      },
    ],
    factory,
    id: "empty-recording",
    schemaVersion: "factory-recording/v1",
    title: "Empty local recording",
  };
}

function playbackRecording(): unknown {
  const factory = {
    name: "support-playback",
    workers: [{ name: "support-agent" }],
    workTypes: [
      {
        name: "support-request",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "resolved", type: "TERMINAL" },
          { name: "failed", type: "FAILED" },
        ],
      },
    ],
    workstations: [
      {
        inputs: [{ state: "queued", workType: "support-request" }],
        name: "triage",
        outputs: [{ state: "resolved", workType: "support-request" }],
        worker: "support-agent",
      },
    ],
  };
  const context = (sequence: number, tick: number, dispatchId?: string) => ({
    ...(dispatchId ? { dispatchId, workIds: ["support-1"] } : {}),
    eventTime: `2026-07-18T20:00:0${sequence}Z`,
    sequence,
    sessionId: "storybook-playback-session",
    sessionSequence: sequence,
    tick,
  });
  return {
    events: [
      {
        context: context(1, 1),
        id: "playback-topology",
        payload: { factory },
        schemaVersion: "agent-factory.event.v1",
        type: "INITIAL_STRUCTURE_REQUEST",
      },
      {
        context: context(2, 3),
        id: "playback-work",
        payload: {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Support request 1",
              workId: "support-1",
              workTypeName: "support-request",
            },
          ],
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_REQUEST",
      },
      {
        context: context(4, 3),
        id: "playback-resolved",
        payload: {
          fromPlaceId: "support-request:failed",
          fromState: "failed",
          source: "api",
          toPlaceId: "support-request:resolved",
          toState: "resolved",
          workId: "support-1",
          workTypeName: "support-request",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_STATE_CHANGE",
      },
      {
        context: context(5, 3, "playback-dispatch"),
        id: "playback-dispatch",
        payload: {
          inputs: [{ workId: "support-1" }],
          resources: [],
          transitionId: "triage",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "DISPATCH_REQUEST",
      },
      {
        context: context(3, 3),
        id: "playback-failed-first",
        payload: {
          fromPlaceId: "support-request:queued",
          fromState: "queued",
          source: "api",
          toPlaceId: "support-request:failed",
          toState: "failed",
          workId: "support-1",
          workTypeName: "support-request",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_STATE_CHANGE",
      },
    ],
    factory,
    id: "same-tick-history-current",
    schemaVersion: "factory-recording/v1",
    title: "Same-tick history and current playback",
  };
}
