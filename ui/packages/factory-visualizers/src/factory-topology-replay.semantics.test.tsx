// @vitest-environment happy-dom

import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import {
  WorkstationGuardType,
  WorkstationKind,
  WorkstationType,
} from "@you-agent-factory/client";
import type { FactoryGraphSource } from "@you-agent-factory/factory-graph";
import type { ComponentType, ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
} from "./factory-topology-replay";

vi.mock("@xyflow/react", () => ({
  Background: () => null,
  Controls: () => null,
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    children,
    nodeTypes,
    nodes,
  }: {
    children: ReactNode;
    nodeTypes: Record<string, ComponentType<{ data: Record<string, unknown> }>>;
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
  }) => (
    <div data-testid="public-replay-renderer">
      {nodes.map((node) => {
        const NodeView = nodeTypes[node.type];
        return NodeView ? <NodeView data={node.data} key={node.id} /> : null;
      })}
      {children}
    </div>
  ),
  useStore: (selector: (state: { transform: [number, number, number] }) => unknown) =>
    selector({ transform: [0, 0, 1] }),
}));

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => `${count} active Dispatches`,
  annotationsHidden: "Annotations hidden",
  annotationsVisible: "Annotations visible",
  empty: "No Factory topology is available.",
  failed: "The Factory topology could not be shown.",
  imageFailed: "The annotation image could not be shown.",
  imageLoading: "Loading annotation image.",
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

describe("FactoryTopologyReplay workstation semantics", () => {
  it("renders a guarded logical move with its public control details", () => {
    render(
      <FactoryTopologyReplay
        messages={messages}
        state={{ source: guardedLogicalMoveSource, status: "ready" }}
      />,
    );

    expect(screen.getByTestId("public-replay-renderer")).toBeVisible();
    expect(screen.getByText("Breaker")).toBeVisible();

    const guardCard = screen.getByText("Breaker");
    expect(guardCard).toHaveAttribute(
      "data-workstation-guard-type",
      "VISIT_COUNT",
    );
  });
});

const guardedLogicalMoveSource = {
  factory: {
    name: "Guarded replay",
    workstations: [
      {
        behavior: WorkstationKind.STANDARD,
        guards: [
          {
            maxVisits: 3,
            type: WorkstationGuardType.VISIT_COUNT,
            workstation: "goal-loop-breaker",
          },
        ],
        id: "goal-loop-breaker",
        inputs: [],
        name: "goal-loop-breaker",
        outputs: [],
        type: WorkstationType.LOGICAL_MOVE,
        worker: "",
      },
    ],
  },
  runtime: {
    activity: {
      activeDispatchOverlays: [],
      activeWorkstationNodeIds: [],
      issues: [],
      resourceOccupancy: [],
      selectedTick: 0,
    },
    load: {
      issues: [],
      resourceOccupancy: [],
      selectedTick: 0,
      workStateCounts: [],
    },
    topology: {
      connections: [],
      issues: [],
      nodes: [
        {
          entityId: "goal-loop-breaker",
          handles: [],
          id: "workstation:goal-loop-breaker",
          kind: "workstation",
          label: "goal-loop-breaker",
        },
      ],
      ok: true,
      selectedTick: 0,
    },
  },
  selectedTick: 0,
} satisfies FactoryGraphSource;
