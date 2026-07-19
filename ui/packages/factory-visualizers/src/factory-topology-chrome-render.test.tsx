import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen } from "@testing-library/react";
import type { FactoryVisualizationLayoutV1 } from "@you-agent-factory/client";
import type { ComponentType } from "react";
import { describe, expect, it, vi } from "vitest";

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
    </fieldset>
  ),
  Handle: () => null,
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
  activeWorkDuration: (ticks) => `Active for ${ticks} ticks`,
  activeWorkOverflow: (count) => `${count} more active Work`,
  activeWorkRegionLabel: "Active Work",
  annotationsHidden: "Show annotations",
  annotationsVisible: "Hide annotations",
  empty: "No Factory topology is available.",
  failed: "The Factory topology could not be shown.",
  imageFailed: "The annotation image could not be shown.",
  imageLoading: "Loading annotation image.",
  inactiveDispatches: "No active Dispatch",
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

describe("FactoryTopologyReplay chrome rendering", () => {
  it.each([
    ["full", true, true, true, true],
    ["minimal", false, true, true, false],
    ["none", false, false, false, false],
  ] as const)(
    "renders only the %s preset chrome",
    (preset, legend, background, viewportControls, visibilityControls) => {
      renderReadyTopology({ preset });

      expectOptionalRegion(
        legend,
        screen.queryByRole("group", { name: messages.legendLabel }),
      );
      expectOptionalRegion(background, screen.queryByTestId("flow-background"));
      expectOptionalRegion(
        viewportControls,
        screen.queryByTestId("flow-controls"),
      );
      expectOptionalRegion(
        visibilityControls,
        screen.queryByRole("button", { name: messages.annotationsVisible }),
      );
    },
  );

  it("applies a chrome override without exposing disabled regions", () => {
    renderReadyTopology({
      legend: true,
      preset: "none",
      visibilityControls: true,
    });

    expect(
      screen.getByRole("group", { name: messages.legendLabel }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: messages.annotationsVisible }),
    ).toBeVisible();
    expect(screen.queryByTestId("flow-background")).not.toBeInTheDocument();
    expect(screen.queryByTestId("flow-controls")).not.toBeInTheDocument();
  });

  it.each(["full", "minimal", "none"] as const)(
    "keeps the loading state explicit with the %s preset",
    (preset) => {
      render(
        <FactoryTopologyReplay
          chrome={{ preset }}
          messages={messages}
          state={{ status: "loading" }}
        />,
      );

      expect(
        screen.getByRole("region", { name: messages.regionLabel }),
      ).toHaveAttribute("aria-busy", "true");
      expect(screen.getByText(messages.loading)).toBeVisible();
      expect(screen.queryByTestId("react-flow")).not.toBeInTheDocument();
    },
  );

  it.each(["full", "minimal", "none"] as const)(
    "retains bounded active Work, resource, Work State, and route evidence with the %s preset",
    (preset) => {
      const projection = createFactoryTopologyProjection();
      projection.activity.selectedTick = 12;
      projection.activity.activeDispatchOverlays[0].startedTick = 7;
      projection.activity.activeDispatchOverlays[0].workIds = [
        "work-4",
        "work-2",
        "work-1",
        "work-3",
      ];
      render(
        <FactoryTopologyReplay
          chrome={{ preset }}
          messages={messages}
          state={{ projection, status: "ready" }}
        />,
      );

      expect(
        screen.getByRole("region", { name: messages.activeWorkRegionLabel }),
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

function expectOptionalRegion(enabled: boolean, region: HTMLElement | null) {
  if (enabled) expect(region).toBeInTheDocument();
  else expect(region).not.toBeInTheDocument();
}

function renderReadyTopology(
  chrome:
    | { preset: "full" | "minimal" | "none" }
    | {
        legend: boolean;
        preset: "full" | "minimal" | "none";
        visibilityControls: boolean;
      },
) {
  render(
    <FactoryTopologyReplay
      chrome={chrome}
      layout={annotationLayout()}
      messages={messages}
      state={{
        projection: createFactoryTopologyProjection(),
        status: "ready",
      }}
    />,
  );
}

function annotationLayout(): FactoryVisualizationLayoutV1 {
  return {
    annotations: [
      {
        body: "Read the validation notes before routing work.",
        id: "review-note",
        kind: "note",
        position: { x: 80, y: 40 },
      },
    ],
    schemaVersion: "factory-visualization-layout/v1",
  };
}
