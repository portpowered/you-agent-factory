import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentType } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
} from "./factory-topology-replay";

const mockFlow = vi.hoisted(() => ({ error: undefined as Error | undefined }));

vi.mock("@xyflow/react", () => ({
  Background: () => null,
  Controls: () => null,
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    nodes,
    nodeTypes,
  }: {
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    nodeTypes: Record<string, ComponentType<{ data: Record<string, unknown> }>>;
  }) => {
    if (mockFlow.error) throw mockFlow.error;
    return nodes.map((node) => {
      const NodeView = nodeTypes[node.type];
      return NodeView ? <NodeView data={node.data} key={node.id} /> : null;
    });
  },
}));

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => `${count} active Dispatches`,
  activeWorkDuration: (ticks) => `${ticks} logical ticks`,
  activeWorkOverflow: (count) => `+${count} active Work`,
  activeWorkRows: (count) => `${count} active Work rows`,
  empty: "No Factory topology is available.",
  failed: "The Factory topology could not be shown.",
  hideNodeKinds: "Hide node kinds",
  inactiveDispatches: "No active Dispatch",
  legendLabel: "Topology legend",
  loading: "Loading Factory topology.",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology replay",
  resourceOccupancy: (occupied, capacity) => `${occupied} of ${capacity}`,
  resourceOccupancyUnavailable: "Resource occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected",
  showNodeKinds: "Show node kinds",
  workStateCount: (count) => `${count} Work`,
  workStateCountUnavailable: "Work count unavailable",
};

describe("FactoryTopologyReplay render containment", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFlow.error = undefined;
  });

  it("contains React Flow failures while sibling host UI remains operational", async () => {
    mockFlow.error = new Error("renderer internals");
    const onError = vi.fn();
    const siblingAction = vi.fn();
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    render(
      <div>
        <button onClick={siblingAction} type="button">
          Host action
        </button>
        <FactoryTopologyReplay
          messages={messages}
          onError={onError}
          state={{ projection: createProjection(), status: "ready" }}
        />
      </div>,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(messages.failed);
    fireEvent.click(screen.getByRole("button", { name: "Host action" }));
    expect(siblingAction).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "react-flow" }),
      ),
    );
    consoleError.mockRestore();
  });

  it("contains non-graph render failures in the labeled region", async () => {
    const brokenMessages = { ...messages };
    Object.defineProperty(brokenMessages, "loading", {
      get() {
        throw new Error("message renderer failure");
      },
    });
    const onError = vi.fn();
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    render(
      <FactoryTopologyReplay
        messages={brokenMessages}
        onError={onError}
        state={{ status: "loading" }}
      />,
    );

    expect(
      screen.getByRole("region", { name: messages.regionLabel }),
    ).toContainElement(screen.getByRole("alert"));
    await waitFor(() =>
      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "render" }),
      ),
    );
    consoleError.mockRestore();
  });
});

function createProjection(): FactoryTopologyReplayProjection {
  return {
    activity: {
      activeDispatchOverlays: [],
      activeWorkstationNodeIds: [],
      issues: [],
      resourceOccupancy: [],
      selectedTick: 1,
    },
    load: {
      issues: [],
      resourceOccupancy: [],
      selectedTick: 1,
      workStateCounts: [],
    },
    topology: {
      connections: [],
      issues: [],
      nodes: [
        {
          entityId: "worker-1",
          handles: [],
          id: "worker:worker-1",
          kind: "worker",
          label: "Worker 1",
        },
      ],
      ok: true,
      selectedTick: 1,
    },
  };
}
