import { describe, expect, it } from "vitest";
import {
  createDefaultFactoryLayout,
  factoryLayoutNodePosition,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";
import {
  addFactoryLayoutGroup,
  createFactoryLayoutGroup,
  defaultFactoryLayoutGroupBounds,
} from "../visual-groups/factory-graph-layout-groups";
import {
  applyFactoryLayoutCommand,
  createCreateFactoryLayoutGroupCommand,
  createMoveFactoryLayoutVisualGroupCommand,
  createUpdateFactoryLayoutGroupCommand,
  type FactoryLayoutCommand,
  invertFactoryLayoutCommand,
} from "./factory-graph-layout-commands";

function requireCommand(
  command: FactoryLayoutCommand | null,
): FactoryLayoutCommand {
  expect(command).not.toBeNull();
  if (!command) {
    throw new Error("Expected layout command to be created.");
  }
  return command;
}

describe("factory graph layout group commands", () => {
  it("creates, renames, and styles groups with undo support", () => {
    const layout = createDefaultFactoryLayout();
    const group = createFactoryLayoutGroup({
      bounds: defaultFactoryLayoutGroupBounds({ x: 120, y: 80 }),
      id: "group-1",
      layout,
      color: "primary",
    });
    const createCommand = createCreateFactoryLayoutGroupCommand({ group });
    const createdLayout = applyFactoryLayoutCommand(layout, createCommand);
    expect(createdLayout.groups).toHaveLength(1);

    const renamedLayout = applyFactoryLayoutCommand(
      createdLayout,
      requireCommand(
        createUpdateFactoryLayoutGroupCommand({
          groupId: "group-1",
          layout: createdLayout,
          to: {
            ...group,
            label: "Planning",
          },
        }),
      ),
    );
    const styledLayout = applyFactoryLayoutCommand(
      renamedLayout,
      requireCommand(
        createUpdateFactoryLayoutGroupCommand({
          groupId: "group-1",
          layout: renamedLayout,
          to: {
            ...group,
            color: "success",
            label: "Planning",
          },
        }),
      ),
    );

    expect(styledLayout.groups?.[0]).toMatchObject({
      color: "success",
      id: "group-1",
      label: "Planning",
    });
    expect(hasFactoryLayoutChanges(layout, styledLayout)).toBe(true);

    const undoneCreate = applyFactoryLayoutCommand(
      createdLayout,
      invertFactoryLayoutCommand(createCommand),
    );
    expect(undoneCreate.groups ?? []).toEqual([]);
  });

  it("avoids noop update commands when group metadata is unchanged", () => {
    const layout = addFactoryLayoutGroup(
      createDefaultFactoryLayout(),
      createFactoryLayoutGroup({
        bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
        id: "group-1",
        layout: createDefaultFactoryLayout(),
      }),
    );
    const group = layout.groups?.[0];
    expect(group).toBeDefined();
    if (!group) {
      throw new Error("Expected group fixture.");
    }

    expect(
      createUpdateFactoryLayoutGroupCommand({
        groupId: "group-1",
        layout,
        to: group,
      }),
    ).toBeNull();
  });
});

describe("factory graph visual group move commands", () => {
  it("moves the group and every member in one undoable command", () => {
    const group = createFactoryLayoutGroup({
      bounds: { height: 240, width: 360, x: 40, y: 60 },
      id: "group-1",
      layout: createDefaultFactoryLayout(),
      nodeIds: ["member-a", "member-b"],
    });
    let layout = addFactoryLayoutGroup(createDefaultFactoryLayout(), group);
    layout = moveFactoryLayoutNode(layout, "member-a", { x: 80, y: 100 });
    layout = moveFactoryLayoutNode(layout, "member-b", { x: 180, y: 140 });
    layout = moveFactoryLayoutNode(layout, "unrelated", { x: 500, y: 520 });

    const command = requireCommand(
      createMoveFactoryLayoutVisualGroupCommand({
        delta: { x: 32, y: -18 },
        groupId: "group-1",
        layout,
      }),
    );
    expect(command.type).toBe("move-visual-group");

    const moved = applyFactoryLayoutCommand(layout, command);
    expect(moved.groups?.[0]?.bounds).toEqual({
      height: 240,
      width: 360,
      x: 72,
      y: 42,
    });
    expect(factoryLayoutNodePosition(moved, "member-a")).toEqual({
      x: 112,
      y: 82,
    });
    expect(factoryLayoutNodePosition(moved, "member-b")).toEqual({
      x: 212,
      y: 122,
    });
    expect(factoryLayoutNodePosition(moved, "unrelated")).toEqual({
      x: 500,
      y: 520,
    });

    const undone = applyFactoryLayoutCommand(
      moved,
      invertFactoryLayoutCommand(command),
    );
    expect(undone.groups?.[0]?.bounds).toEqual(group.bounds);
    expect(factoryLayoutNodePosition(undone, "member-a")).toEqual({
      x: 80,
      y: 100,
    });
    expect(factoryLayoutNodePosition(undone, "member-b")).toEqual({
      x: 180,
      y: 140,
    });
    expect(factoryLayoutNodePosition(undone, "unrelated")).toEqual({
      x: 500,
      y: 520,
    });

    const redone = applyFactoryLayoutCommand(undone, command);
    expect(redone).toEqual(moved);
  });
});
