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
  Controls: ({ "aria-label": ariaLabel }: { "aria-label"?: string }) => (
    <fieldset data-testid="flow-controls">
      <legend>{ariaLabel}</legend>
      <button type="button">Zoom in</button>
    </fieldset>
  ),
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
  annotationsHidden: "Show annotations",
  annotationsVisible: "Hide annotations",
  empty: "No Factory topology is available.",
  failed: "The Factory topology could not be shown.",
  inactiveDispatches: "No active Dispatch",
  imageFailed: "The annotation image could not be shown.",
  imageLoading: "Loading annotation image.",
  legendActiveRoute: "Active route",
  legendInactiveRoute: "Inactive route",
  legendLabel: "Topology legend",
  loading: "Loading Factory topology.",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology replay",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} resources occupied`,
  resourceOccupancyUnavailable: "Resource occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected",
  viewportControlsLabel: "Topology viewport controls",
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
      index < 4 && workstation !== document.activeElement;
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

  it("keeps enabled controls named and keyboard reachable while unmounting disabled chrome", async () => {
    const user = userEvent.setup();
    render(
      <FactoryTopologyReplay
        chrome={{ legend: false, preset: "full", visibilityControls: false }}
        messages={messages}
        state={{
          projection: createFactoryTopologyProjection(),
          status: "ready",
        }}
      />,
    );

    expect(screen.getByTestId("flow-background")).toBeInTheDocument();
    expect(
      screen.getByRole("group", { name: messages.viewportControlsLabel }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: messages.legendLabel }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: messages.annotationsVisible }),
    ).not.toBeInTheDocument();

    await user.tab();
    expect(screen.getByRole("button", { name: "Zoom in" })).toHaveFocus();
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
