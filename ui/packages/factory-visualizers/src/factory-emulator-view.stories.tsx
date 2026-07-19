import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";

import { FactoryEmulatorView } from "./factory-emulator-view";
import { createFactoryTopologyProjection } from "./testing/factory-topology-projection";

const meta = {
  title: "Factory Visualizers/FactoryEmulatorView",
  component: FactoryEmulatorView,
  args: {
    controls: {
      formatTick: String,
      isPlaying: false,
      onFollowLatest: () => undefined,
      onPause: () => undefined,
      onPlay: () => undefined,
      onRestart: () => undefined,
      onSelectTick: () => undefined,
      onSpeedChange: () => undefined,
      onStep: () => undefined,
      runtimeStatus: { label: "Ready", tone: "success" },
      speed: 1,
      timeline: {
        messages: {
          alreadyFollowingLatest: "Following latest.",
          currentMode: "Current Factory.",
          disabled: "Timeline disabled.",
          followLatest: "Follow latest",
          historyMode: "Viewing history.",
          position: (selected: string, latest: string) =>
            `Tick ${selected} of ${latest}`,
          regionLabel: "Factory timeline",
          sliderLabel: "Select tick",
          title: "Timeline",
          unavailable: "No timeline.",
        },
        state: {
          earliestTick: 0,
          latestTick: 2,
          mode: "current",
          selectedTick: 2,
          status: "available",
        },
      },
    },
    submission: <button type="button">Submit Work</button>,
    topology: {
      messages: {
        activeDispatches: (count: number) => `${count} active Dispatches`,
        empty: "No topology.",
        failed: "Topology failed.",
        inactiveDispatches: "No active Dispatches",
        loading: "Loading topology.",
        nodeLabel: (kind: string, label: string) => `${kind}: ${label}`,
        regionLabel: "Factory topology",
        resourceOccupancy: (occupied: number, capacity: number) =>
          `${occupied}/${capacity}`,
        resourceOccupancyUnavailable: "Occupancy unavailable",
        retry: "Retry",
        selectedNode: "Selected",
        workStateCount: (count: number) => `${count} Work`,
        workStateCountUnavailable: "Work unavailable",
      },
      state: { projection: createFactoryTopologyProjection(), status: "ready" },
    },
    workProgress: {
      formatNumber: String,
      messages: {
        categories: {
          active: { plural: String, singular: String },
          completed: { plural: String, singular: String },
          failed: { plural: String, singular: String },
          queued: { plural: String, singular: String },
          unclassified: { plural: String, singular: String },
        },
        empty: "No Work.",
        regionLabel: "Work progress",
        title: "Work progress",
        total: (count: string) => `${count} Work total`,
      },
      projection: {
        active: [],
        completed: [],
        failed: [],
        queued: [],
        unclassified: [],
        counts: {
          active: 0,
          completed: 0,
          failed: 0,
          queued: 0,
          unclassified: 0,
        },
        selectedTick: 2,
        total: 0,
      },
    },
  },
  parameters: { layout: "padded" },
} satisfies Meta<typeof FactoryEmulatorView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Full: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Play" })).toBeVisible();
    await expect(
      canvas.getByRole("region", { name: "Factory topology" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Submit Work" }),
    ).toBeVisible();
  },
};
export const Compact: Story = { args: { preset: "compact" } };
export const DisplayOnly: Story = { args: { preset: "display-only" } };
