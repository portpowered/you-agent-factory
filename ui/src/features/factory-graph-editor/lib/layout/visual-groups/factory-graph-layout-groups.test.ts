import { describe, expect, it } from "vitest";
import {
  createDefaultFactoryLayout,
  factoryLayoutNodePosition,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";
import {
  addFactoryLayoutGroup,
  addNodeToFactoryLayoutGroup,
  clampFactoryLayoutGroupBounds,
  createFactoryLayoutGroup,
  createFactoryLayoutGroupId,
  defaultFactoryLayoutGroupBounds,
  factoryLayoutGroupById,
  factoryLayoutGroupCanvasNodeOptions,
  factoryLayoutGroupColorCssVariable,
  factoryLayoutGroupColorSurfaceCssVariable,
  factoryLayoutGroupContainsNode,
  factoryLayoutGroups,
  factoryLayoutGroupsEqual,
  isApprovedFactoryLayoutGroupColor,
  moveFactoryLayoutGroupByDelta,
  removeFactoryLayoutGroup,
  removeNodeFromAllFactoryLayoutGroups,
  removeNodeFromFactoryLayoutGroup,
  resizeFactoryLayoutGroup,
  updateFactoryLayoutGroup,
} from "./factory-graph-layout-groups";

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
    expect(
      factoryLayoutNodePosition(withoutGroup, "workstation:draft"),
    ).toEqual({
      x: 40,
      y: 60,
    });
    expect(withoutGroup.edges).toEqual(withMember.edges);
  });

  it("no-ops when adding an existing member or removing a missing member", () => {
    const layout = addNodeToFactoryLayoutGroup(
      addFactoryLayoutGroup(
        createDefaultFactoryLayout(),
        createFactoryLayoutGroup({
          bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
          id: "group-1",
          layout: createDefaultFactoryLayout(),
        }),
      ),
      "group-1",
      "workstation:draft",
    );

    const unchangedAdd = addNodeToFactoryLayoutGroup(
      layout,
      "group-1",
      "workstation:draft",
    );
    const unchangedRemove = removeNodeFromFactoryLayoutGroup(
      layout,
      "group-1",
      "worker:writer",
    );

    expect(factoryLayoutGroupById(unchangedAdd, "group-1")?.nodeIds).toEqual([
      "workstation:draft",
    ]);
    expect(factoryLayoutGroupById(unchangedRemove, "group-1")?.nodeIds).toEqual(
      ["workstation:draft"],
    );
  });

  it("returns the original layout when moving a missing group or empty member set", () => {
    const layout = addFactoryLayoutGroup(
      createDefaultFactoryLayout(),
      createFactoryLayoutGroup({
        bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
        id: "group-1",
        layout: createDefaultFactoryLayout(),
      }),
    );

    expect(
      moveFactoryLayoutGroupByDelta(layout, "missing-group", { x: 1, y: 2 }),
    ).toBe(layout);
    expect(
      moveFactoryLayoutGroupByDelta(layout, "group-1", { x: 1, y: 2 })
        .groups?.[0]?.bounds,
    ).toEqual({
      height: 320,
      width: 480,
      x: -239,
      y: -158,
    });
  });

  it("maps approved and fallback group colors to css variables", () => {
    expect(factoryLayoutGroupColorCssVariable("success")).toBe(
      "var(--color-success)",
    );
    expect(factoryLayoutGroupColorCssVariable("unknown")).toBe(
      "var(--color-primary)",
    );
    expect(factoryLayoutGroupColorSurfaceCssVariable("outline")).toBe(
      "var(--color-surface-container-low)",
    );
    expect(factoryLayoutGroupColorSurfaceCssVariable("primary")).toBe(
      "var(--color-primary-container)",
    );
    expect(factoryLayoutGroupColorSurfaceCssVariable("warning")).toBe(
      "var(--color-warning-container)",
    );
    expect(factoryLayoutGroupColorSurfaceCssVariable(undefined)).toBe(
      "var(--color-primary-container)",
    );
  });

  it("clamps undersized group bounds during resize", () => {
    expect(
      clampFactoryLayoutGroupBounds({
        height: 10,
        width: 20,
        x: 5,
        y: 6,
      }),
    ).toEqual({
      height: 80,
      width: 120,
      x: 5,
      y: 6,
    });
  });

  it("removes a node from other groups when reassigning membership", () => {
    const layout = addFactoryLayoutGroup(
      addFactoryLayoutGroup(createDefaultFactoryLayout(), {
        bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
        id: "group-1",
        label: "Lane A",
        nodeIds: ["workstation:draft"],
      }),
      {
        bounds: defaultFactoryLayoutGroupBounds({ x: 120, y: 0 }),
        id: "group-2",
        label: "Lane B",
        nodeIds: [],
      },
    );

    const withoutNode = removeNodeFromAllFactoryLayoutGroups(
      layout,
      "workstation:draft",
      "group-2",
    );

    expect(factoryLayoutGroupById(withoutNode, "group-1")?.nodeIds).toEqual([]);
    expect(factoryLayoutGroupById(withoutNode, "group-2")?.nodeIds).toEqual([]);
  });

  it("allocates the next available group id when defaults collide", () => {
    const layout = addFactoryLayoutGroup(createDefaultFactoryLayout(), {
      bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
      id: "group-1",
      label: "Existing",
      nodeIds: [],
    });

    expect(createFactoryLayoutGroupId(layout)).toBe("group-2");
  });

  it("detects equal groups and no-ops updates for missing groups", () => {
    const group = createFactoryLayoutGroup({
      bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
      id: "group-1",
      layout: createDefaultFactoryLayout(),
    });

    expect(factoryLayoutGroupsEqual(group, structuredClone(group))).toBe(true);
    expect(
      updateFactoryLayoutGroup(
        createDefaultFactoryLayout(),
        "missing",
        (current) => ({
          ...current,
          label: "Ignored",
        }),
      ),
    ).toEqual(createDefaultFactoryLayout());
  });

  it("reads groups from layout metadata helpers", () => {
    const layout = addFactoryLayoutGroup(createDefaultFactoryLayout(), {
      bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
      id: "group-1",
      label: "Review",
      nodeIds: [],
    });

    expect(factoryLayoutGroups(layout)).toHaveLength(1);
    expect(factoryLayoutGroupById(layout, "group-1")?.label).toBe("Review");
    expect(
      factoryLayoutGroupById(createDefaultFactoryLayout(), "missing"),
    ).toBe(undefined);
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
