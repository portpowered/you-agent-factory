import { describe, expect, it } from "vitest";
import {
  createDefaultFactoryLayout,
  hasFactoryLayoutChanges,
} from "../factory-graph-layout-operations";
import {
  addFactoryLayoutGroup,
  createFactoryLayoutGroup,
  defaultFactoryLayoutGroupBounds,
} from "../visual-groups/factory-graph-layout-groups";
import {
  applyFactoryLayoutCommand,
  createCreateFactoryLayoutGroupCommand,
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
