import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReactFlowProvider } from "@xyflow/react";

import { GraphNodeButton, type GraphNodeHandle, GraphNodeShell } from "./index";

const genericHandles: GraphNodeHandle[] = [
  {
    id: "input-target",
    label: "Input",
    side: "left",
    tone: "input",
    type: "target",
  },
  {
    id: "output-source",
    label: "Output",
    side: "right",
    tone: "output",
    type: "source",
  },
];

function GraphNodeStateExample({
  buttonLabel,
  graphState,
  shellState,
  stateLabel,
}: {
  buttonLabel: string;
  graphState?: "default" | "selected" | "disabled" | "loading" | "error";
  shellState?: "default" | "selected" | "disabled" | "loading" | "error";
  stateLabel?: string;
}) {
  return (
    <ReactFlowProvider>
      <div className="w-72">
        <GraphNodeShell
          handles={genericHandles}
          nodeKind="example"
          state={shellState ?? graphState ?? "default"}
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
  title: "Graphs/GraphNodeStates",
  component: GraphNodeStateExample,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof GraphNodeStateExample>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Selected: Story = {
  args: {
    buttonLabel: "Selected node",
    graphState: "selected",
    shellState: "selected",
    stateLabel: "Selected node",
  },
};

export const Disabled: Story = {
  args: {
    buttonLabel: "Disabled node",
    graphState: "disabled",
    shellState: "disabled",
    stateLabel: "Disabled node",
  },
};

export const Loading: Story = {
  args: {
    buttonLabel: "Loading node",
    graphState: "loading",
    shellState: "loading",
    stateLabel: "Loading node",
  },
};

export const Loaded: Story = {
  args: {
    buttonLabel: "Loading node",
    graphState: "default",
    shellState: "default",
  },
};

export const ErrorState: Story = {
  args: {
    buttonLabel: "Error node",
    graphState: "error",
    shellState: "error",
    stateLabel: "Connection failed",
  },
};
