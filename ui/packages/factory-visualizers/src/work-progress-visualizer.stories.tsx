import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FactoryWorkProgressProjection } from "@you-agent-factory/factory-replay";
import { expect, within } from "storybook/test";

import {
  WorkProgressVisualizer,
  type WorkProgressVisualizerMessages,
} from "./work-progress-visualizer";

const messages: WorkProgressVisualizerMessages = {
  categories: {
    queued: {
      singular: (count) => `${count} queued Work`,
      plural: (count) => `${count} queued Work`,
    },
    active: {
      singular: (count) => `${count} active Work`,
      plural: (count) => `${count} active Work`,
    },
    completed: {
      singular: (count) => `${count} completed Work`,
      plural: (count) => `${count} completed Work`,
    },
    failed: {
      singular: (count) => `${count} failed Work`,
      plural: (count) => `${count} failed Work`,
    },
    unclassified: {
      singular: (count) => `${count} unclassified Work`,
      plural: (count) => `${count} unclassified Work`,
    },
  },
  empty: "No Work is present at this replay tick.",
  regionLabel: "Factory work progress",
  title: "Work progress",
  total: (count) => `${count} Work total`,
};

const projection: FactoryWorkProgressProjection = {
  active: [{ id: "active-1" }, { id: "active-2" }],
  completed: [{ id: "completed-1" }, { id: "completed-2" }],
  counts: {
    active: 2,
    completed: 2,
    failed: 1,
    queued: 3,
    unclassified: 1,
  },
  failed: [{ id: "failed-1" }],
  queued: [{ id: "queued-1" }, { id: "queued-2" }, { id: "queued-3" }],
  selectedTick: 18,
  total: 9,
  unclassified: [{ id: "unclassified-1" }],
};

const meta = {
  title: "Factory Visualizers/WorkProgressVisualizer",
  component: WorkProgressVisualizer,
  args: {
    formatNumber: new Intl.NumberFormat("en-US").format,
    messages,
    projection,
  },
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof WorkProgressVisualizer>;

export default meta;

type Story = StoryObj<typeof meta>;

function responsiveStory(width: string): Story {
  return {
    decorators: [
      (Story) => (
        <div data-story-width={width} style={{ maxWidth: "100%", width }}>
          <Story />
        </div>
      ),
    ],
    play: async ({ canvasElement }) => {
      const canvas = within(canvasElement);
      const region = await canvas.findByRole("region", {
        name: "Factory work progress",
      });
      const shell =
        canvasElement.querySelector<HTMLElement>("[data-story-width]");

      await expect(region).toHaveAttribute("data-work-progress-total", "9");
      await expect(canvas.getByText("3 queued Work")).toBeVisible();
      await expect(canvas.getByText("1 failed Work")).toBeVisible();
      expect(shell).not.toBeNull();
      expect((shell?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(
        true,
      );
    },
  };
}

export const Small: Story = responsiveStory("20rem");
export const Medium: Story = responsiveStory("42rem");
export const Large: Story = responsiveStory("70rem");

export const Empty: Story = {
  args: {
    projection: {
      active: [],
      completed: [],
      counts: {
        active: 0,
        completed: 0,
        failed: 0,
        queued: 0,
        unclassified: 0,
      },
      failed: [],
      queued: [],
      selectedTick: 0,
      total: 0,
      unclassified: [],
    },
  },
};
