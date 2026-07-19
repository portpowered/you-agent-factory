import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen } from "@testing-library/react";
import type { ComponentType } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
} from "./factory-topology-replay";
import { createFactoryTopologyProjection } from "./testing/factory-topology-projection";

vi.mock("@xyflow/react", () => ({
  Background: () => null,
  Controls: () => null,
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    edges,
    nodes,
    nodeTypes,
  }: {
    edges: Array<{ animated?: boolean; id: string }>;
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
          data-animated={edge.animated ? "true" : "false"}
          data-edge-id={edge.id}
          key={edge.id}
        />
      ))}
    </div>
  ),
}));

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => `${count} active Dispatches`,
  annotationsHidden: "Show annotations",
  annotationsVisible: "Hide annotations",
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

describe("FactoryTopologyReplay operational detail", () => {
  it.each(["full", "minimal", "none"] as const)(
    "retains bounded Work rows and runtime evidence with the %s preset",
    (preset) => {
      const projection = createFactoryTopologyProjection();
      projection.activity.selectedTick = 12;
      projection.activity.activeDispatchOverlays[0].startedTick = 7;
      render(
        <FactoryTopologyReplay
          chrome={{ preset }}
          messages={messages}
          state={{ projection, status: "ready" }}
        />,
      );

      expect(
        screen.getByRole("group", { name: "Active Work" }),
      ).toHaveTextContent("work-1");
      expect(screen.getAllByText("Active for 5 ticks")).toHaveLength(3);
      expect(screen.getByText("1 more active Work")).toBeVisible();
      expect(screen.getByText(/2 of 4 resources occupied/)).toBeVisible();
      expect(screen.getByText(/3 Work in this state/)).toBeVisible();
      expect(
        document.querySelector('[data-edge-id="worker-assignment"]'),
      ).toHaveAttribute("data-animated", "true");
    },
  );
});
