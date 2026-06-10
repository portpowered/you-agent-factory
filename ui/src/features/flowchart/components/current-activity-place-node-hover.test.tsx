import { cleanup, render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import type { StatePositionNodeData } from "./current-activity-place-node";
import { StatePositionNodeView } from "./current-activity-place-node";

const CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS =
  "transition-[background-color,border-color,box-shadow,opacity] hover:border-primary hover:bg-warning-container hover:opacity-100 hover:shadow-af-accent-chip";

vi.mock("@xyflow/react", () => ({
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  Position: { Left: "left", Right: "right" },
}));

function statePositionNodeProps(
  place: DashboardPlaceRef,
  overrides: Partial<StatePositionNodeData> = {},
): NodeProps<{
  data: StatePositionNodeData;
  id: string;
  type: "statePosition";
}> {
  return {
    data: {
      activeFlow: false,
      activeItemLabels: [],
      handles: [],
      muted: false,
      place,
      selectedStateNode: false,
      tokenCount: 0,
      ...overrides,
    },
    dragging: false,
    id: place.place_id,
    isConnectable: false,
    selected: false,
    type: "statePosition",
    zIndex: 0,
  };
}

function nodeShell(container: HTMLElement): HTMLElement | null {
  return container.querySelector(
    "[data-current-activity-node-type='statePosition']",
  );
}

describe("CurrentActivity place node hover emphasis", () => {
  afterEach(() => {
    cleanup();
  });

  it("applies accent hover classes on neutral work-state nodes", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:ready",
      state_category: "INITIAL",
      state_value: "ready",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView {...statePositionNodeProps(place)} />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).toContain("hover:border-primary");
    expect(shell?.className).toContain("hover:bg-warning-container");
    expect(shell?.className).toContain(CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS);
  });

  it("keeps accent hover classes when active-flow or muted", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:ready",
      state_category: "INITIAL",
      state_value: "ready",
      type_id: "story",
    };

    for (const overrides of [{ activeFlow: true }, { muted: true }] as const) {
      const { container } = render(
        <StatePositionNodeView {...statePositionNodeProps(place, overrides)} />,
      );
      const shell = nodeShell(container);

      expect(shell?.className).toContain("hover:border-primary");
      expect(shell?.className).toContain("hover:bg-warning-container");
    }
  });

  it("suppresses accent hover classes when selected or validation-error", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:ready",
      state_category: "INITIAL",
      state_value: "ready",
      type_id: "story",
    };

    for (const overrides of [
      { selectedStateNode: true },
      { onSelectStateNode: () => undefined, validationError: true },
    ] as const) {
      const { container } = render(
        <StatePositionNodeView {...statePositionNodeProps(place, overrides)} />,
      );
      const shell = nodeShell(container);

      expect(shell?.className).not.toContain("hover:border-primary");
    }
  });
});
