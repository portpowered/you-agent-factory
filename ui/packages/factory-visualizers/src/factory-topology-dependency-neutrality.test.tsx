import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen } from "@testing-library/react";
import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import type { ComponentType } from "react";
import { describe, expect, it, vi } from "vitest";

import { projectFactoryTopologyAtTick } from "../../factory-replay/src/index.js";
import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
} from "./factory-topology-replay";

vi.mock("@xyflow/react", () => ({
  Background: () => null,
  Controls: () => <div data-testid="flow-controls" />,
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    children,
    edges,
    nodes,
    nodeTypes,
  }: {
    children: React.ReactNode;
    edges: Array<{ id: string }>;
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    nodeTypes: Record<string, ComponentType<{ data: Record<string, unknown> }>>;
  }) => (
    <div data-testid="react-flow">
      {nodes.map((node) => {
        const NodeView = nodeTypes[node.type];
        return NodeView ? <NodeView data={node.data} key={node.id} /> : null;
      })}
      {edges.map((edge) => (
        <span data-edge-id={edge.id} key={edge.id} />
      ))}
      {children}
    </div>
  ),
}));

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => `${count} active Dispatches`,
  annotationsHidden: "Show annotations",
  annotationsVisible: "Hide annotations",
  empty: "No Factory topology is available.",
  failed: "The Factory topology could not be shown.",
  inactiveDispatches: "No active Dispatch",
  loading: "Loading Factory topology.",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology replay",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} resources occupied`,
  resourceOccupancyUnavailable: "Resource occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected",
  workStateCount: (count) => `${count} Work in this state`,
  workStateCountUnavailable: "Work count unavailable",
};

const dependencyFactory = {
  name: "dependency-neutral-topology",
  workTypes: [
    {
      name: "task",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "complete", type: "TERMINAL" },
      ],
    },
  ],
} satisfies FactoryDefinition;

const relation = {
  requiredState: "complete",
  sourceWorkName: "dependent",
  targetWorkId: "prerequisite-id",
  targetWorkName: "prerequisite",
  type: "DEPENDS_ON",
};

const dependencyEvents: FactoryEvent[] = [
  {
    context: { eventTime: "2026-07-18T12:00:00Z", sequence: 1, tick: 0 },
    id: "structure",
    payload: { factory: dependencyFactory },
    schemaVersion: "agent-factory.event.v1",
    type: "INITIAL_STRUCTURE_REQUEST",
  },
  {
    context: {
      eventTime: "2026-07-18T12:00:01Z",
      sequence: 2,
      tick: 1,
      workIds: ["dependent-id", "prerequisite-id"],
    },
    id: "work-request",
    payload: {
      relations: [relation],
      works: [
        { id: "dependent-id", name: "dependent" },
        { id: "prerequisite-id", name: "prerequisite" },
      ],
    },
    schemaVersion: "agent-factory.event.v1",
    type: "WORK_REQUEST",
  },
  {
    context: {
      eventTime: "2026-07-18T12:00:02Z",
      sequence: 3,
      tick: 1,
      workIds: ["dependent-id", "prerequisite-id"],
    },
    id: "relationship-change",
    payload: { relation },
    schemaVersion: "agent-factory.event.v1",
    type: "RELATIONSHIP_CHANGE_REQUEST",
  },
];

describe("FactoryTopologyReplay dependency neutrality", () => {
  it("keeps DEPENDS_ON replay evidence out of the rendered topology", () => {
    const topology = projectFactoryTopologyAtTick({
      events: dependencyEvents,
      tick: 1,
    });
    const projection: FactoryTopologyReplayProjection = {
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
      topology,
    };

    render(
      <FactoryTopologyReplay
        messages={messages}
        state={{ projection, status: "ready" }}
      />,
    );

    expect(topology.connections.map(({ kind }) => kind)).toEqual([
      "work-type-state",
      "work-type-state",
    ]);
    expect(document.querySelectorAll("[data-edge-id]")).toHaveLength(2);
    expect(document.body).not.toHaveTextContent(
      /depends_on|dependency|blocked/i,
    );
    expect(
      document.querySelector('[data-dispatch-activity="active"]'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: /relationship|dependency|connect/i,
      }),
    ).not.toBeInTheDocument();
  });
});
