import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { NodeProps } from "@xyflow/react";
import { describe, expect, it, vi } from "vitest";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import {
  type CurrentActivityWorkTypeNode,
  WorkTypeNodeView,
} from "./current-activity-work-type-node";

const place: DashboardPlaceRef = {
  kind: "constraint",
  place_id: "work-type:story",
  state_value: "story",
};

function renderWorkTypeNode(
  data: Partial<NodeProps<CurrentActivityWorkTypeNode>["data"]> = {},
) {
  return render(
    <WorkTypeNodeView
      data={{
        activeFlow: false,
        handles: [],
        kind: "work-type",
        muted: false,
        place,
        ...data,
      }}
      dragging={false}
      id="work-type:story"
      selected={false}
      type="workType"
      zIndex={0}
    />,
  );
}

describe("WorkTypeNodeView", () => {
  it("renders a non-interactive presentation when selection is unsupported", () => {
    renderWorkTypeNode();

    expect(screen.getByRole("img", { name: "work-type:story" })).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Select story work type" }),
    ).toBeNull();
  });

  it("selects work types from click and keyboard activation", async () => {
    const user = userEvent.setup();
    const onSelectWorkType = vi.fn();

    renderWorkTypeNode({
      onSelectWorkType,
      selectedWorkType: false,
    });

    const button = screen.getByRole("button", {
      name: "Select story work type",
    });
    expect(button.getAttribute("aria-pressed")).toBe("false");

    await user.click(button);
    expect(onSelectWorkType).toHaveBeenCalledWith("story");

    button.focus();
    await user.keyboard("{Enter}");
    expect(onSelectWorkType).toHaveBeenCalledTimes(2);
  });

  it("exposes selected state when the work type is active", () => {
    renderWorkTypeNode({
      onSelectWorkType: vi.fn(),
      selectedWorkType: true,
    });

    expect(
      screen
        .getByRole("button", { name: "Select story work type" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("renders the work-type identity projection after resize", () => {
    const { container } = renderWorkTypeNode({ expanded: true });

    expect(
      container.querySelector(
        '[data-factory-graph-expanded-content="work-type"]',
      ),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-factory-graph-expanded-field="place-id"]')
        ?.textContent,
    ).toBe("work-type:story");
  });
});
