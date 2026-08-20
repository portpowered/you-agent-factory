import { cleanup, render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSurfaceClassName,
} from "../../factory-graph-editor/lib/work-state/factory-graph-work-state-phase-styling";
import { activityGraphNodeSurfaceClassName } from "./current-activity-node-chrome";
import type { StatePositionNodeData } from "./current-activity-place-node";
import { StatePositionNodeView } from "./current-activity-place-node";

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

describe("CurrentActivity place node work-state phase styling", () => {
  afterEach(() => {
    cleanup();
  });

  it.each([
    ["INITIAL", "waiting", "border-info-border bg-info-container"],
    ["PROCESSING", "active", "border-af-warning-border bg-warning-container"],
    ["TERMINAL", "success", "border-af-success-border bg-success-container"],
    ["FAILED", "danger", "border-af-danger-border bg-error-container"],
  ] as const)(
    "applies lifecycle surface classes for work_state with state_category %s",
    (stateCategory, status, surfaceClass) => {
      const place: DashboardPlaceRef = {
        kind: "work_state",
        place_id: `story:${stateCategory.toLowerCase()}`,
        state_category: stateCategory,
        state_value: stateCategory.toLowerCase(),
        type_id: "story",
      };
      const { container } = render(
        <StatePositionNodeView {...statePositionNodeProps(place)} />,
      );
      const shell = nodeShell(container);

      expect(shell?.className).toContain(
        workStatePhaseSurfaceClassName(stateCategory),
      );
      expect(shell?.getAttribute("data-graph-visual-status")).toBe(status);
      expect(shell?.getAttribute("data-graph-visual-surface")).toBe(status);
      for (const className of surfaceClass.split(" ")) {
        expect(shell?.className).toContain(className);
      }
    },
  );

  it("uses neutral surface styling when work_state has no state_category", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:unknown",
      state_value: "unknown",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView {...statePositionNodeProps(place)} />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).toContain(
      workStatePhaseSurfaceClassName(undefined),
    );
  });

  it("keeps resource node styling unchanged", () => {
    const place: DashboardPlaceRef = {
      kind: "resource",
      place_id: "resource:cpu",
      state_value: "cpu",
      type_id: "resource",
    };
    const { container } = render(
      <StatePositionNodeView {...statePositionNodeProps(place)} />,
    );
    const shell = container.querySelector(
      "[data-current-activity-node-type='resource']",
    );

    expect(shell?.className).toContain(
      activityGraphNodeSurfaceClassName("resource"),
    );
    expect(shell?.className).not.toContain(
      activityGraphNodeSurfaceClassName("info"),
    );
  });

  it("applies lifecycle icon tone from shared phase styling", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:processing",
      state_category: "PROCESSING",
      state_value: "processing",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView {...statePositionNodeProps(place)} />,
    );
    const icon = container.querySelector("[data-place-semantic-icon] svg");

    expect(icon?.getAttribute("class")).toContain(
      workStatePhaseSemanticIconClassName("PROCESSING"),
    );
  });
});

describe("CurrentActivity place node work-state phase precedence", () => {
  afterEach(() => {
    cleanup();
  });

  it("keeps selection ring visible on lifecycle-colored work-state nodes", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:ready",
      state_category: "INITIAL",
      state_value: "ready",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(place, { selectedStateNode: true })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).toContain("border-primary");
    expect(shell?.getAttribute("data-graph-visual-border")).toBe("selection");
    expect(shell?.getAttribute("data-graph-visual-selection")).toBe("true");
    expect(shell?.className).toContain(
      workStatePhaseSurfaceClassName("INITIAL"),
    );
  });

  it("keeps validation error ring visible on lifecycle-colored work-state nodes", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:failed",
      state_category: "FAILED",
      state_value: "failed",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(place, {
          onSelectStateNode: () => undefined,
          validationError: true,
        })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).toContain("shadow-af-graph-validation-danger");
    expect(shell?.className).toContain("!border-error");
    expect(shell?.getAttribute("data-graph-visual-border")).toBe("validation");
    expect(shell?.getAttribute("data-graph-visual-validation")).toBe("error");
    expect(shell?.className).toContain(
      workStatePhaseSurfaceClassName("FAILED"),
    );
  });

  it("applies active-flow border over idle phase surface styling", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:ready",
      state_category: "PROCESSING",
      state_value: "ready",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(place, { activeFlow: true })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).toContain("border-af-warning-border");
    expect(shell?.className).toContain("shadow-af-graph-warning");
    expect(shell?.getAttribute("data-graph-visual-emphasis")).toBe("strong");
    expect(shell?.getAttribute("data-graph-visual-treatment")).toBe(
      "processing",
    );
    expect(shell?.className).toContain(
      workStatePhaseSurfaceClassName("PROCESSING"),
    );
  });

  it("keeps idle phase surface classes when muted", () => {
    const place: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:complete",
      state_category: "TERMINAL",
      state_value: "complete",
      type_id: "story",
    };
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(place, { muted: true })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).toContain(
      workStatePhaseSurfaceClassName("TERMINAL"),
    );
    expect(shell?.className).toContain("agent-flow-node--muted");
    expect(shell?.className).not.toContain("opacity-[0.45]");
  });
});

/** `!bg-warning` is a substring of `!bg-warning-container`; compare tokens. */
function shellClassTokens(shell: HTMLElement | null): string[] {
  return (shell?.className ?? "").split(" ").filter(Boolean);
}

function workStatePlace(stateCategory: "PROCESSING" | "TERMINAL") {
  return {
    kind: "work_state",
    place_id: `story:${stateCategory.toLowerCase()}`,
    state_category: stateCategory,
    state_value: stateCategory.toLowerCase(),
    type_id: "story",
  } satisfies DashboardPlaceRef;
}

describe("CurrentActivity work-state node fill by held work", () => {
  afterEach(() => {
    cleanup();
  });

  it("keeps an empty processing work state translucent", () => {
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(workStatePlace("PROCESSING"), {
          tokenCount: 0,
        })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.getAttribute("data-graph-visual-fill")).toBe("soft");
    expect(shellClassTokens(shell)).toContain("!bg-warning-container");
    expect(shellClassTokens(shell)).not.toContain("!bg-warning");
  });

  it("keeps an empty processing work state on default label ink", () => {
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(workStatePlace("PROCESSING"), {
          tokenCount: 0,
        })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).not.toContain(
      "[&_[data-state-value]]:!text-on-warning",
    );
    expect(
      container.querySelector("[data-state-value]")?.getAttribute("class"),
    ).toContain("text-on-surface");
  });

  it("fills a processing work state solidly once work is in it", () => {
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(workStatePlace("PROCESSING"), {
          tokenCount: 2,
        })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.getAttribute("data-graph-visual-fill")).toBe("solid");
    expect(shellClassTokens(shell)).toContain("!bg-warning");
    expect(shellClassTokens(shell)).not.toContain("!bg-warning-container");
  });

  it("inverts work-state label ink on a solid fill", () => {
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(workStatePlace("PROCESSING"), {
          tokenCount: 2,
        })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.className).toContain(
      "[&_[data-state-value]]:!text-on-warning",
    );
    expect(shell?.className).toContain(
      "[&_[data-state-work-type]]:!text-on-warning",
    );
  });

  it("inverts the work-state icon on a solid fill so it stays visible", () => {
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(workStatePlace("PROCESSING"), {
          tokenCount: 2,
        })}
      />,
    );
    const shell = nodeShell(container);

    expect(container.querySelector("[data-graph-semantic-icon]")).toBeTruthy();
    expect(shell?.className).toContain(
      "[&_[data-graph-semantic-icon]]:!text-on-warning",
    );
  });

  it("fills a held terminal work state solidly in its own tone", () => {
    const { container } = render(
      <StatePositionNodeView
        {...statePositionNodeProps(workStatePlace("TERMINAL"), {
          tokenCount: 1,
        })}
      />,
    );
    const shell = nodeShell(container);

    expect(shell?.getAttribute("data-graph-visual-fill")).toBe("solid");
    expect(shellClassTokens(shell)).toContain("!bg-success");
    expect(shell?.className).toContain(
      "[&_[data-state-value]]:!text-on-success",
    );
  });
});
