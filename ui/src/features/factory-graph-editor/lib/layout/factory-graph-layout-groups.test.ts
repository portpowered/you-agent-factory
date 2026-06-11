import { describe, expect, it } from "vitest";

import {
  addFactoryLayoutGroup,
  createFactoryLayoutGroup,
  createFactoryLayoutGroupId,
  defaultFactoryLayoutGroupBounds,
  factoryLayoutGroupById,
  isApprovedFactoryLayoutGroupColor,
  updateFactoryLayoutGroup,
} from "./factory-graph-layout-groups";
import {
  createDefaultFactoryLayout,
  hasFactoryLayoutChanges,
} from "./factory-graph-layout-operations";

describe("factory graph layout groups", () => {
  it("creates stable group ids and finite default bounds", () => {
    const layout = createDefaultFactoryLayout();
    const groupId = createFactoryLayoutGroupId(layout);
    const group = createFactoryLayoutGroup({
      bounds: defaultFactoryLayoutGroupBounds({ x: 400, y: 300 }),
      id: groupId,
      layout,
    });

    expect(groupId).toBe("group-1");
    expect(group.bounds).toEqual({
      height: 320,
      width: 480,
      x: 160,
      y: 140,
    });
    expect(group.nodeIds).toEqual([]);
    expect(group.label).toBe("Group 1");
    expect(isApprovedFactoryLayoutGroupColor(group.color)).toBe(true);
  });

  it("updates group label and color without changing topology metadata", () => {
    const baseLayout = addFactoryLayoutGroup(
      createDefaultFactoryLayout(),
      createFactoryLayoutGroup({
        bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
        id: "group-1",
        layout: createDefaultFactoryLayout(),
      }),
    );
    const renamedLayout = updateFactoryLayoutGroup(
      baseLayout,
      "group-1",
      (group) => ({
        ...group,
        label: "Review lane",
      }),
    );
    const styledLayout = updateFactoryLayoutGroup(
      renamedLayout,
      "group-1",
      (group) => ({
        ...group,
        color: "info",
      }),
    );

    expect(factoryLayoutGroupById(styledLayout, "group-1")).toEqual({
      bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
      color: "info",
      id: "group-1",
      label: "Review lane",
      nodeIds: [],
    });
    expect(hasFactoryLayoutChanges(baseLayout, styledLayout)).toBe(true);
  });
});
