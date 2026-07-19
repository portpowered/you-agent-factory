import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";

import { FactoryEmulatorControls } from "./factory-emulator-controls";

const meta = {
  title: "Factory Visualizers/FactoryEmulatorControls",
  component: FactoryEmulatorControls,
  args: {
    formatTick: String,
    isPlaying: false,
    onFollowLatest: fn(),
    onPause: fn(),
    onPlay: fn(),
    onRestart: fn(),
    onSelectTick: fn(),
    onSpeedChange: fn(),
    onStep: fn(),
    runtimeStatus: { label: "Viewing history", tone: "warning" },
    speed: 1,
    timeline: {
      messages: {
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
      },
      state: {
        earliestTick: 0,
        latestTick: 8,
        mode: "history",
        selectedTick: 3,
        status: "available",
      },
    },
  },
  parameters: { layout: "padded" },
} satisfies Meta<typeof FactoryEmulatorControls>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ViewingHistory: Story = {};

export const Current: Story = {
  args: {
    runtimeStatus: { label: "Running", tone: "success" },
    timeline: {
      ...meta.args.timeline,
      state: {
        earliestTick: 0,
        latestTick: 8,
        mode: "current",
        selectedTick: 8,
        status: "available",
      },
    },
  },
};
