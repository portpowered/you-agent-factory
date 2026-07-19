import { fireEvent, render, screen } from "@testing-library/react";
import type { ComponentType } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
  projectFactoryTopologyFlow,
} from "./factory-topology-replay";

vi.mock("@xyflow/react", () => ({
  Background: () => <div data-testid="flow-background" />,
  Controls: () => <div data-testid="flow-controls" />,
  Handle: ({ id, type }: { id: string; type: string }) => (
    <span data-handle-id={id} data-handle-role={type} />
  ),
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    children,
    edges,
    nodes,
    nodeTypes,
  }: {
    children: React.ReactNode;
    edges: Array<{ id: string; sourceHandle?: string; targetHandle?: string }>;
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    nodeTypes: Record<string, ComponentType<{ data: Record<string, unknown> }>>;
  }) => (
    <div data-testid="react-flow">
      {nodes.map((node) => {
        const NodeView = nodeTypes[node.type];
        return NodeView ? <NodeView data={node.data} key={node.id} /> : null;
      })}
      {edges.map((edge) => (
        <span
          data-edge-id={edge.id}
          data-source-handle={edge.sourceHandle}
          data-target-handle={edge.targetHandle}
          key={edge.id}
        />
      ))}
      {children}
    </div>
  ),
}));

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => `${count} active Dispatches`,
  inactiveDispatches: "No active Dispatch",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology replay",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} resources occupied`,
  resourceOccupancyUnavailable: "Resource occupancy unavailable",
  selectedNode: "Selected",
  workStateCount: (count) => `${count} Work in this state`,
  workStateCountUnavailable: "Work count unavailable",
};

describe("FactoryTopologyReplay", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders semantic endpoints and selected-tick activity and load evidence", () => {
    const projection = createProjection();
    render(
      <FactoryTopologyReplay messages={messages} projection={projection} />,
    );

    expect(
      screen.getByRole("region", { name: messages.regionLabel }),
    ).toHaveAttribute("data-endpoints-valid", "true");
    expect(screen.getAllByText(/1 active Dispatches/).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText(/2 of 4 resources occupied/)).toBeVisible();
    expect(screen.getByText(/3 Work in this state/)).toBeVisible();
    expect(screen.getAllByText(/No active Dispatch/).length).toBeGreaterThan(0);
    expect(
      document.querySelector('[data-edge-id="worker-assignment"]'),
    ).toHaveAttribute("data-source-handle", "worker-assignment-source");
    expect(
      document.querySelector('[data-edge-id="worker-assignment"]'),
    ).toHaveAttribute("data-target-handle", "worker-assignment-target");
    expect(
      document.querySelector(
        '[data-handle-id="worker-assignment-source"][data-handle-role="source"]',
      ),
    ).not.toBeNull();
    expect(
      document.querySelector(
        '[data-handle-id="worker-assignment-target"][data-handle-role="target"]',
      ),
    ).not.toBeNull();
  });

  it("fails closed without sending a partially valid edge set to React Flow", () => {
    const projection = createProjection();
    projection.topology.connections.push({
      ...projection.topology.connections[0],
      id: "invalid-edge",
      target: { handleId: "missing-handle", nodeId: "workstation:review" },
    });

    const flow = projectFactoryTopologyFlow(
      projection,
      messages,
      undefined,
      undefined,
    );
    expect(flow.validEndpoints).toBe(false);
    expect(flow.edges).toEqual([]);

    render(
      <FactoryTopologyReplay messages={messages} projection={projection} />,
    );
    expect(document.querySelector("[data-edge-id]")).toBeNull();
  });
});

describe("FactoryTopologyReplay controlled updates", () => {
  it("emits selection intent while selection styling remains host-controlled", () => {
    const projection = createProjection();
    const original = structuredClone(projection);
    const onSelectNode = vi.fn();
    const { rerender } = render(
      <FactoryTopologyReplay
        messages={messages}
        onSelectNode={onSelectNode}
        projection={projection}
      />,
    );
    const workstation = screen.getByRole("button", {
      name: "workstation: Review",
    });

    fireEvent.click(workstation);

    expect(onSelectNode).toHaveBeenCalledWith(projection.topology.nodes[3]);
    expect(screen.queryByText("Selected")).not.toBeInTheDocument();
    expect(workstation).not.toHaveAttribute("aria-pressed", "true");
    expect(projection).toEqual(original);

    rerender(
      <FactoryTopologyReplay
        messages={messages}
        onSelectNode={onSelectNode}
        projection={projection}
        selectedNodeId="workstation:review"
      />,
    );
    expect(screen.getByText(/Selected/)).toBeVisible();
    expect(
      screen.getByRole("button", { name: "workstation: Review" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("replaces activity overlays without changing stable topology identities", () => {
    const first = createProjection();
    const second = createProjection();
    second.activity = {
      ...second.activity,
      activeDispatchOverlays: [],
      selectedTick: 9,
    };
    second.topology = { ...second.topology, selectedTick: 9 };
    second.load = { ...second.load, selectedTick: 9 };
    const firstFlow = projectFactoryTopologyFlow(
      first,
      messages,
      undefined,
      undefined,
    );
    const secondFlow = projectFactoryTopologyFlow(
      second,
      messages,
      undefined,
      undefined,
    );
    const { rerender } = render(
      <FactoryTopologyReplay messages={messages} projection={first} />,
    );

    expect(screen.getAllByText(/1 active Dispatches/).length).toBeGreaterThan(
      0,
    );
    rerender(<FactoryTopologyReplay messages={messages} projection={second} />);

    expect(screen.queryByText(/1 active Dispatches/)).not.toBeInTheDocument();
    expect(screen.getAllByText(/No active Dispatch/)).toHaveLength(4);
    expect(secondFlow.nodes.map(({ id }) => id)).toEqual(
      firstFlow.nodes.map(({ id }) => id),
    );
    expect(secondFlow.edges.map(({ id }) => id)).toEqual(
      firstFlow.edges.map(({ id }) => id),
    );
  });
});

function createProjection(): FactoryTopologyReplayProjection {
  return {
    activity: {
      activeDispatchOverlays: [
        {
          connectionIds: ["worker-assignment"],
          dispatchId: "dispatch-1",
          evidence: {
            resources: "known",
            route: "known",
            work: "known",
            worker: "known",
            workstation: "known",
          },
          id: "overlay:dispatch-1",
          resourceNodeIds: ["resource:gpu"],
          startedTick: 7,
          workerNodeId: "worker:alice",
          workstationNodeId: "workstation:review",
        },
      ],
      activeWorkstationNodeIds: ["workstation:review"],
      issues: [],
      resourceOccupancy: [],
      selectedTick: 8,
    },
    load: {
      issues: [],
      resourceOccupancy: [
        {
          availableQuantity: 2,
          capacity: 4,
          capacityEvidence: "known",
          evidence: "known",
          occupiedQuantity: 2,
          resourceId: "gpu",
          resourceNodeId: "resource:gpu",
        },
      ],
      selectedTick: 8,
      workStateCounts: [
        {
          count: 3,
          evidence: "known",
          workStateId: "queued",
          workStateNodeId: "work-state:task:queued",
          workTypeId: "task",
        },
      ],
    },
    topology: {
      connections: [
        {
          id: "worker-assignment",
          kind: "worker-assignment",
          source: {
            handleId: "worker-assignment-source",
            nodeId: "worker:alice",
          },
          target: {
            handleId: "worker-assignment-target",
            nodeId: "workstation:review",
          },
        },
      ],
      issues: [],
      nodes: [
        {
          capacity: 4,
          entityId: "gpu",
          handles: [],
          id: "resource:gpu",
          kind: "resource",
          label: "GPU",
        },
        {
          entityId: "alice",
          handles: [{ id: "worker-assignment-source", role: "source" }],
          id: "worker:alice",
          kind: "worker",
          label: "Alice",
        },
        {
          category: "INITIAL",
          entityId: "task:queued",
          handles: [],
          id: "work-state:task:queued",
          kind: "work-state",
          label: "Queued",
          workTypeId: "task",
        },
        {
          entityId: "review",
          handles: [{ id: "worker-assignment-target", role: "target" }],
          id: "workstation:review",
          kind: "workstation",
          label: "Review",
        },
      ],
      ok: true,
      selectedTick: 8,
    },
  };
}
