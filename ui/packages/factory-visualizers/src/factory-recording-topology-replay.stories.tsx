import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import customerSupportRecording from "../../client/examples/customer-support.factory-recording.v1.json";

import {
  FactoryRecordingTopologyReplay,
  type FactoryRecordingTopologyReplayMessages,
} from "./factory-recording-topology-replay";

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
  topology: {
    activeDispatches: (count) =>
      `${count} active ${count === 1 ? "Dispatch" : "Dispatches"}`,
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
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof FactoryRecordingTopologyReplay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ValidatedRecording: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("region", { name: messages.regionLabel }),
    ).toHaveAttribute("data-selected-tick", "0");
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
  },
};

function category(name: string) {
  return {
    plural: (count: string) => `${count} ${name} Work`,
    singular: (count: string) => `${count} ${name} Work`,
  };
}
