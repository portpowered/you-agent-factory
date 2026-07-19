import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";

import {
  FactoryTimelineScrubber,
  type FactoryTimelineScrubberMessages,
} from "./factory-timeline-scrubber";

const messages: FactoryTimelineScrubberMessages = {
  alreadyFollowingLatest: "Following the latest Factory tick.",
  currentMode: "Showing the current Factory.",
  disabled: "Timeline selection is disabled by the host.",
  followLatest: "Follow latest",
  historyMode: "Showing Factory history.",
  position: (selected, latest) => `Tick ${selected} of ${latest}`,
  regionLabel: "Factory replay timeline",
  sliderLabel: "Select replay tick",
  title: "Replay timeline",
  unavailable: "No replay ticks are available.",
};

const meta = {
  title: "Factory Visualizers/FactoryTimelineScrubber",
  component: FactoryTimelineScrubber,
  args: {
    formatTick: new Intl.NumberFormat("en-US").format,
    messages,
    onFollowLatest: fn(),
    onSelectTick: fn(),
    state: {
      earliestTick: 0,
      latestTick: 24,
      mode: "history",
      selectedTick: 8,
      status: "available",
    },
  },
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof FactoryTimelineScrubber>;

export default meta;

type Story = StoryObj<typeof meta>;

export const HistoryKeyboard: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const slider = canvas.getByRole("slider", { name: "Select replay tick" });

    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}");

    await expect(slider).toHaveFocus();
    await expect(args.onSelectTick).toHaveBeenCalledWith(9);
    await expect(canvas.getByText("Tick 8 of 24")).toBeVisible();
  },
};

export const FollowingLatest: Story = {
  args: {
    state: {
      earliestTick: 0,
      latestTick: 24,
      mode: "current",
      selectedTick: 24,
      status: "available",
    },
  },
};

export const Unavailable: Story = {
  args: {
    state: { status: "unavailable" },
  },
};

export const DisabledByHost: Story = {
  args: {
    disabled: true,
  },
};

export const GermanHistory: Story = {
  args: {
    formatTick: new Intl.NumberFormat("de-DE").format,
    messages: {
      alreadyFollowingLatest: "Der neueste Fabrikschritt wird angezeigt.",
      currentMode: "Die aktuelle Fabrik wird angezeigt.",
      disabled: "Die Zeitleistenauswahl wurde deaktiviert.",
      followLatest: "Neuestem Schritt folgen",
      historyMode: "Der Fabrikverlauf wird angezeigt.",
      position: (selected, latest) => `Schritt ${selected} von ${latest}`,
      regionLabel: "Zeitleiste der Fabrikwiedergabe",
      sliderLabel: "Wiedergabeschritt auswählen",
      title: "Wiedergabezeitleiste",
      unavailable: "Keine Wiedergabeschritte verfügbar.",
    },
    state: {
      earliestTick: 0,
      latestTick: 12000,
      mode: "history",
      selectedTick: 7000,
      status: "available",
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Schritt 7.000 von 12.000")).toBeVisible();
    await expect(
      canvas.getByRole("slider", {
        name: "Wiedergabeschritt auswählen",
      }),
    ).toHaveAttribute("aria-valuetext", "7.000");
  },
};
