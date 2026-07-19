import type { Meta, StoryObj } from "@storybook/react-vite";

import { FactoryEmulatorControls } from "./factory-emulator-controls";

const meta = {
  title: "Factory emulator/Controls",
  component: FactoryEmulatorControls,
  args: {
    isPlaying: false,
    onPause: () => undefined,
    onPlay: () => undefined,
    onRestart: () => undefined,
    onSpeedChange: () => undefined,
    onStep: () => undefined,
    runtimeStatus: { label: "Ready", tone: "success" },
    speed: 1,
  },
} satisfies Meta<typeof FactoryEmulatorControls>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {};

export const Running: Story = {
  args: {
    isPlaying: true,
    runtimeStatus: { label: "Running", tone: "success" },
    speed: 2,
  },
};

export const ActionsUnavailable: Story = {
  args: {
    disabledActions: ["play", "step"],
    runtimeStatus: { label: "Waiting for input", tone: "warning" },
  },
};
