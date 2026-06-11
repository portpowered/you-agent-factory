import { describe, expect, it } from "vitest";

import {
  addFactoryLayoutGroup,
  addNodeToFactoryLayoutGroup,
  createFactoryLayoutGroup,
  createFactoryLayoutGroupId,
  defaultFactoryLayoutGroupBounds,
  factoryLayoutGroupById,
  factoryLayoutGroupCanvasNodeOptions,
  factoryLayoutGroupContainsNode,
  isApprovedFactoryLayoutGroupColor,
  moveFactoryLayoutGroupByDelta,
  removeFactoryLayoutGroup,
  removeNodeFromFactoryLayoutGroup,
  resizeFactoryLayoutGroup,
  updateFactoryLayoutGroup,
} from "./factory-graph-layout-groups";
import {
  createDefaultFactoryLayout,
  factoryLayoutNodePosition,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: group layout scenarios share fixtures.
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

  it("assigns and removes group members on the group entry without topology changes", () => {
    const baseLayout = addFactoryLayoutGroup(
      createDefaultFactoryLayout(),
      createFactoryLayoutGroup({
        bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
        id: "group-1",
        layout: createDefaultFactoryLayout(),
      }),
    );
    const withMember = addNodeToFactoryLayoutGroup(
      baseLayout,
      "group-1",
      "workstation:draft",
    );

    const group = factoryLayoutGroupById(withMember, "group-1");
    expect(group).toBeDefined();
    expect(factoryLayoutGroupContainsNode(group, "workstation:draft")).toBe(
      true,
    );
    expect(withMember.nodes).toEqual(baseLayout.nodes);
    expect(withMember.edges).toEqual(baseLayout.edges);

    const withoutMember = removeNodeFromFactoryLayoutGroup(
      withMember,
      "group-1",
      "workstation:draft",
    );

    expect(factoryLayoutGroupById(withoutMember, "group-1")?.nodeIds).toEqual(
      [],
    );
  });

  it("moves flat group membership to a single group and preserves parentGroupId", () => {
    const layout = {
      ...addFactoryLayoutGroup(
        addFactoryLayoutGroup(createDefaultFactoryLayout(), {
          bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
          color: "primary",
          id: "group-1",
          label: "Lane A",
          nodeIds: ["workstation:draft"],
          parentGroupId: "parent-a",
        }),
        {
          bounds: defaultFactoryLayoutGroupBounds({ x: 120, y: 0 }),
          color: "info",
          id: "group-2",
          label: "Lane B",
          nodeIds: [],
        },
      ),
    };
    const reassigned = addNodeToFactoryLayoutGroup(
      layout,
      "group-2",
      "workstation:draft",
    );

    expect(factoryLayoutGroupById(reassigned, "group-1")?.nodeIds).toEqual([]);
    expect(factoryLayoutGroupById(reassigned, "group-1")?.parentGroupId).toBe(
      "parent-a",
    );
    expect(factoryLayoutGroupById(reassigned, "group-2")?.nodeIds).toEqual([
      "workstation:draft",
    ]);
  });

  it("moves group bounds and member nodes by the same delta", () => {
    const layout = moveFactoryLayoutNode(
      addFactoryLayoutGroup(
        createDefaultFactoryLayout(),
        createFactoryLayoutGroup({
          bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
          id: "group-1",
          layout: createDefaultFactoryLayout(),
        }),
      ),
      "workstation:draft",
      { x: 40, y: 60 },
    );
    const withMember = addNodeToFactoryLayoutGroup(
      layout,
      "group-1",
      "workstation:draft",
    );
    const moved = moveFactoryLayoutGroupByDelta(withMember, "group-1", {
      x: 20,
      y: 10,
    });

    expect(factoryLayoutGroupById(moved, "group-1")?.bounds).toEqual({
      height: 320,
      width: 480,
      x: -220,
      y: -150,
    });
    expect(factoryLayoutNodePosition(moved, "workstation:draft")).toEqual({
      x: 60,
      y: 70,
    });
    expect(moved.edges).toEqual(withMember.edges);
  });

  it("resizes group bounds without moving member nodes", () => {
    const layout = moveFactoryLayoutNode(
      addFactoryLayoutGroup(
        createDefaultFactoryLayout(),
        createFactoryLayoutGroup({
          bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
          id: "group-1",
          layout: createDefaultFactoryLayout(),
        }),
      ),
      "workstation:draft",
      { x: 40, y: 60 },
    );
    const withMember = addNodeToFactoryLayoutGroup(
      layout,
      "group-1",
      "workstation:draft",
    );
    const resized = resizeFactoryLayoutGroup(withMember, "group-1", {
      height: 200,
      width: 300,
      x: 10,
      y: 20,
    });

    expect(factoryLayoutGroupById(resized, "group-1")?.bounds).toEqual({
      height: 200,
      width: 300,
      x: 10,
      y: 20,
    });
    expect(factoryLayoutNodePosition(resized, "workstation:draft")).toEqual({
      x: 40,
      y: 60,
    });
  });

  it("deletes only the group entry and preserves member nodes", () => {
    const layout = moveFactoryLayoutNode(
      addFactoryLayoutGroup(
        createDefaultFactoryLayout(),
        createFactoryLayoutGroup({
          bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
          id: "group-1",
          layout: createDefaultFactoryLayout(),
        }),
      ),
      "workstation:draft",
      { x: 40, y: 60 },
    );
    const withMember = addNodeToFactoryLayoutGroup(
      layout,
      "group-1",
      "workstation:draft",
    );
    const withoutGroup = removeFactoryLayoutGroup(withMember, "group-1");

    expect(withoutGroup.groups).toBeUndefined();
    expect(factoryLayoutNodePosition(withoutGroup, "workstation:draft")).toEqual({
      x: 40,
      y: 60,
    });
    expect(withoutGroup.edges).toEqual(withMember.edges);
  });

  it("builds sorted canvas node options from topology nodes", () => {
    expect(
      factoryLayoutGroupCanvasNodeOptions([
        {
          id: "workstation:review",
          key: { kind: "workstation", name: "review" },
          kind: "workstation",
          label: "Review",
        },
        {
          id: "worker:writer",
          key: { kind: "worker", name: "writer" },
          kind: "worker",
          label: "Writer",
        },
      ]),
    ).toEqual([
      { id: "workstation:review", label: "Review" },
      { id: "worker:writer", label: "Writer" },
    ]);
  });
});
