import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";
import type { ComponentType } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
} from "./factory-topology-replay";
import { createFactoryTopologyProjection } from "./testing/factory-topology-projection";

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
      {children}
    </div>
  ),
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
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} resources occupied`,
  resourceOccupancyUnavailable: "Resource occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected",
  showNodeKinds: "Show node kinds",
  workStateCount: (count) => `${count} Work in this state`,
  workStateCountUnavailable: "Work count unavailable",
};

describe("FactoryTopologyReplay inclusive interaction", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("removes active-edge motion when the user prefers reduced motion", () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => ({
        addEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
        matches: true,
        media: "(prefers-reduced-motion: reduce)",
        onchange: null,
        removeEventListener: vi.fn(),
      })),
    );

    render(
      <FactoryTopologyReplay
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );

    expect(screen.getByRole("region")).toHaveAttribute(
      "data-reduced-motion",
      "true",
    );
    expect(
      document.querySelector('[data-edge-id="worker-assignment"]'),
    ).toHaveAttribute("data-animated", "false");
  });

  it("allows keyboard users to activate selectable nodes", async () => {
    const user = userEvent.setup();
    const onSelectNode = vi.fn();
    render(
      <FactoryTopologyReplay
        messages={messages}
        onSelectNode={onSelectNode}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );
    const workstation = screen.getByRole("button", {
      name: "workstation: Review",
    });

    for (
      let index = 0;
      index < 5 && workstation !== document.activeElement;
      index++
    ) {
      await user.tab();
    }
    expect(workstation).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(onSelectNode).toHaveBeenCalledWith(
      expect.objectContaining({ id: "workstation:review" }),
    );
  });

  it("keeps disabled chrome out of the accessibility tree and lets keyboard users operate visibility controls", async () => {
    const user = userEvent.setup();
    render(
      <FactoryTopologyReplay
        chrome={{ preset: "none", visibilityControls: true }}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );

    expect(
      screen.queryByLabelText(messages.legendLabel),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId("flow-background")).not.toBeInTheDocument();
    expect(screen.queryByTestId("flow-controls")).not.toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: messages.hideNodeKinds });
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    expect(
      screen.getByRole("button", { name: messages.showNodeKinds }),
    ).toHaveFocus();
    expect(
      screen.queryByText("workstation", { exact: true }),
    ).not.toBeInTheDocument();
  });

  it("has no automated accessibility violations in the ready state", async () => {
    const { container } = render(
      <FactoryTopologyReplay
        messages={messages}
        onSelectNode={vi.fn()}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
