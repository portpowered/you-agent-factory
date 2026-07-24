import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReactFlowProvider } from "@xyflow/react";

import { GraphInteractiveExample } from "./graph-interactive-example";
import {
  desktopInteractiveGraphNodes,
  genericGraphHandles,
  narrowInteractiveGraphNodes,
} from "./graph-story-fixtures";
import { GraphNodeButton, GraphNodeShell } from "./index";

function GraphNodeStatePanel({
  buttonLabel,
  graphState,
  shellState,
  stateLabel,
  viewportWidthClass = "w-72",
}: {
  buttonLabel: string;
  graphState: "default" | "selected" | "disabled" | "loading" | "error";
  shellState?: "default" | "selected" | "disabled" | "loading" | "error";
  stateLabel?: string;
  viewportWidthClass?: string;
}) {
  return (
    <ReactFlowProvider>
      <div className={viewportWidthClass}>
        <GraphNodeShell
          handles={genericGraphHandles}
          nodeKind="example"
          state={shellState ?? graphState}
          stateLabel={stateLabel}
        >
          <GraphNodeButton graphState={graphState} stateLabel={stateLabel}>
            {buttonLabel}
          </GraphNodeButton>
        </GraphNodeShell>
      </div>
    </ReactFlowProvider>
  );
}

const meta = {
  title: "Graphs/GraphInteractiveExamples",
  component: GraphInteractiveExample,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof GraphInteractiveExample>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Interactive: Story = {
  args: {
    "aria-label": "Interactive graph example",
    className: "h-[28rem]",
    fixtureNodes: desktopInteractiveGraphNodes,
    initialSelectedNodeId: null,
    viewportWidthClass: "w-[48rem] max-w-full",
  },
};

export const Selected: Story = {
  args: {
    fixtureNodes: desktopInteractiveGraphNodes,
    initialSelectedNodeId: "ready-node",
    viewportWidthClass: "w-[48rem] max-w-full",
  },
};

export const Disabled: Story = {
  render: () => (
    <GraphNodeStatePanel
      buttonLabel="Disabled node"
      graphState="disabled"
      shellState="disabled"
      stateLabel="Disabled node"
    />
  ),
};

export const Loading: Story = {
  render: () => (
    <GraphNodeStatePanel
      buttonLabel="Loading node"
      graphState="loading"
      shellState="loading"
      stateLabel="Loading node"
    />
  ),
};

export const ErrorState: Story = {
  render: () => (
    <GraphNodeStatePanel
      buttonLabel="Error node"
      graphState="error"
      shellState="error"
      stateLabel="Connection failed"
    />
  ),
};

export const DesktopViewport: Story = {
  args: {
    className: "h-[30rem]",
    fixtureNodes: desktopInteractiveGraphNodes,
    viewportWidthClass: "w-[48rem] max-w-full",
  },
};

export const NarrowViewport: Story = {
  args: {
    className: "h-[42rem]",
    fixtureNodes: narrowInteractiveGraphNodes,
    viewportWidthClass: "w-80 max-w-full overflow-x-hidden",
  },
};
