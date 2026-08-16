import { render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { describe, expect, it, vi } from "vitest";

import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import { WorkstationKind } from "../../../../api/generated/openapi";
import {
  type CurrentActivityWorkstationNode,
  WorkstationNodeView,
} from "../../../graphs/components/workstation-node-view";

vi.mock("@xyflow/react", () => ({
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  Position: { Left: "left", Right: "right" },
}));

const workstation: DashboardWorkstationNode = {
  node_id: "workstation:draft",
  workstation_kind: WorkstationKind.STANDARD,
  workstation_name: "Draft",
};

function renderWorkstationNode(
  executions: NodeProps<CurrentActivityWorkstationNode>["data"]["executions"],
) {
  return render(
    <WorkstationNodeView
      data={{
        active: executions.length > 0,
        activeFlow: executions.length > 0,
        executions,
        handles: [],
        muted: false,
        now: Date.parse("2026-06-09T00:00:00Z"),
        selectedWorkID: null,
        selectedWorkstation: false,
        workstation,
        onSelectWorkID: vi.fn(),
      }}
      dragging={false}
      id="workstation:draft"
      selected={false}
      type="workstation"
      zIndex={0}
    />,
  );
}

function activeExecution(index: number) {
  return {
    dispatch_id: `dispatch-${index}`,
    started_at: "2026-06-09T00:00:00Z",
    work_items: [
      {
        display_name: `Story ${index}`,
        work_id: `work-${index}`,
      },
    ],
  };
}

describe("CurrentActivity workstation work density", () => {
  it("keeps two active work rows and omits the aggregate marker", () => {
    const view = renderWorkstationNode([
      activeExecution(1),
      activeExecution(2),
    ]);

    expect(view.getByRole("button", { name: /Story 1/ })).toBeTruthy();
    expect(view.getByRole("button", { name: /Story 2/ })).toBeTruthy();
    expect(
      view.container.querySelector("[data-workstation-work-progress]"),
    ).toBe(null);
  });

  it("collapses three active work items to a title-sized count", () => {
    const view = renderWorkstationNode([
      activeExecution(1),
      activeExecution(2),
      activeExecution(3),
    ]);
    const count = view.getByRole("status", { name: "3 active items" });
    const title = view.container.querySelector("[data-workstation-title]");

    expect(count.textContent).toBe("3");
    expect(count.getAttribute("data-workstation-work-progress")).toBe(
      "numeric",
    );
    expect(count.className).toContain("text-base");
    expect(title?.className).toContain("text-[1rem]");
    expect(view.container.querySelector("[data-active-work-label]")).toBe(null);
    expect(view.container.querySelector("[data-active-work-duration]")).toBe(
      null,
    );
    expect(view.queryByRole("button", { name: /Story / })).toBeNull();
  });
});
