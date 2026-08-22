import { cleanup, render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it } from "vitest";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import {
  type CurrentActivityStatePositionNode,
  type StatePositionNodeData,
  StatePositionNodeView,
} from "./current-activity-place-node";

function statePositionNodeProps(
  place: DashboardPlaceRef,
  overrides: Partial<StatePositionNodeData> = {},
): NodeProps<CurrentActivityStatePositionNode> {
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

function expandedField(container: HTMLElement, field: string): string {
  return (
    container.querySelector(`[data-factory-graph-expanded-field="${field}"]`)
      ?.textContent ?? ""
  );
}

describe("CurrentActivity place node expanded content", () => {
  afterEach(() => {
    cleanup();
  });

  it("projects active work details for a resized work-state node", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:processing",
      state_category: "PROCESSING",
      state_value: "processing",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(place, {
          activeItemLabels: ["Review README"],
          expanded: true,
          tokenCount: 1,
        })}
      />,
    );

    expect(
      container.querySelector(
        '[data-factory-graph-expanded-content="work-state"]',
      ),
    ).toBeTruthy();
    expect(expandedField(container, "active-items")).toBe("Review README");
  });

  it.each([
    ["resource", "resource:cpu"],
    ["constraint", "constraint:max"],
  ] as const)(
    "projects the %s identity for a resized place node",
    (family, placeID) => {
      const place: DashboardPlaceRef = {
        kind: family,
        place_id: placeID,
        state_value: family === "resource" ? "cpu" : "max",
        type_id: family === "resource" ? "resource" : "limit",
      };
      const { container } = render(
        <StatePositionNodeView
          {...statePositionNodeProps(place, { expanded: true })}
        />,
      );

      expect(
        container.querySelector(
          `[data-factory-graph-expanded-content="${family}"]`,
        ),
      ).toBeTruthy();
      expect(expandedField(container, "place-id")).toBe(placeID);
    },
  );
});
