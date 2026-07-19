import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
      <button type="button">Zoom in</button>
    </fieldset>
  ),
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="react-flow">{children}</div>
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
    "renders only the %s preset regions",
    (preset, legend, background, viewportControls, visibilityControls) => {
      renderTopology({ preset });

      expectRegion(legend, () =>
        screen.queryByRole("group", { name: messages.legendLabel }),
      );
      expectRegion(background, () => screen.queryByTestId("flow-background"));
      expectRegion(viewportControls, () =>
        screen.queryByRole("group", { name: messages.viewportControlsLabel }),
      );
      expectRegion(visibilityControls, () =>
        screen.queryByRole("button", { name: messages.annotationsVisible }),
      );
    },
  );

  it("applies overrides and keeps disabled controls out of keyboard access", async () => {
    const user = userEvent.setup();
    renderTopology({ background: false, legend: true, preset: "minimal" });

    expect(
      screen.getByRole("group", { name: messages.legendLabel }),
    ).toBeVisible();
    expect(screen.queryByTestId("flow-background")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Zoom in" })).toBeVisible();
    expect(
      screen.queryByRole("button", { name: messages.annotationsVisible }),
    ).not.toBeInTheDocument();

    await user.tab();
    expect(screen.getByRole("button", { name: "Zoom in" })).toHaveFocus();
  });

  it("keeps loading explicit without mounting optional chrome", () => {
    render(
      <FactoryTopologyReplay
        chrome={{ preset: "full" }}
        messages={messages}
        state={{ status: "loading" }}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent(messages.loading);
    expect(screen.queryByTestId("flow-background")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: messages.legendLabel }),
    ).not.toBeInTheDocument();
  });
});

function renderTopology(
  chrome: Parameters<typeof FactoryTopologyReplay>[0]["chrome"],
) {
  render(
    <FactoryTopologyReplay
      chrome={chrome}
      layout={{
        annotations: [
          {
            body: "Read this annotation.",
            id: "annotation",
            kind: "note",
            position: { x: 0, y: 0 },
          },
        ],
        schemaVersion: "factory-visualization-layout/v1",
      }}
      messages={messages}
      state={{ projection: createFactoryTopologyProjection(), status: "ready" }}
    />,
  );
}

function expectRegion(visible: boolean, query: () => HTMLElement | null) {
  if (visible) expect(query()).toBeVisible();
  else expect(query()).not.toBeInTheDocument();
}
