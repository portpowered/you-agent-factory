import type { components } from "../../../../../api/generated/openapi";
import type { FactoryGraphNode } from "../../draft/factory-graph-draft-types";
import type {
  FactoryLayout,
  FactoryLayoutPoint,
} from "../factory-graph-layout-operations";
import { moveFactoryLayoutNodesByDelta } from "../factory-graph-layout-operations";

export type FactoryLayoutGroupCanvasNodeOption = {
  id: string;
  label: string;
};

export type FactoryLayoutGroup = NonNullable<
  components["schemas"]["Factory"]["layout"]
>["groups"] extends (infer TGroup)[] | undefined
  ? TGroup
  : never;

export const FACTORY_LAYOUT_GROUP_DEFAULT_SIZE = {
  height: 320,
  width: 480,
} as const;

export const FACTORY_LAYOUT_GROUP_MIN_SIZE = {
  height: 80,
  width: 120,
} as const;

export const FACTORY_LAYOUT_GROUP_COLOR_TOKENS = [
  "primary",
  "info",
  "success",
  "warning",
  "outline",
] as const;

export type FactoryLayoutGroupColorToken =
  (typeof FACTORY_LAYOUT_GROUP_COLOR_TOKENS)[number];

const FACTORY_LAYOUT_GROUP_COLOR_TOKEN_SET = new Set<string>(
  FACTORY_LAYOUT_GROUP_COLOR_TOKENS,
);

export function isApprovedFactoryLayoutGroupColor(
  color: string | undefined,
): color is FactoryLayoutGroupColorToken {
  return color !== undefined && FACTORY_LAYOUT_GROUP_COLOR_TOKEN_SET.has(color);
}

export function factoryLayoutGroupColorCssVariable(
  color: string | undefined,
): string {
  if (!isApprovedFactoryLayoutGroupColor(color)) {
    return "var(--color-primary)";
  }

  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `var(--color-${color})`;
}

export function factoryLayoutGroupColorSurfaceCssVariable(
  color: string | undefined,
): string {
  if (color === "outline") {
    return "var(--color-surface-container-low)";
  }
  if (!isApprovedFactoryLayoutGroupColor(color)) {
    return "var(--color-primary-container)";
  }

  if (color === "primary") {
    return "var(--color-primary-container)";
  }

  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `var(--color-${color}-container)`;
}

export function factoryLayoutGroups(
  layout: FactoryLayout,
): readonly FactoryLayoutGroup[] {
  return layout.groups ?? [];
}

export function factoryLayoutGroupById(
  layout: FactoryLayout,
  groupId: string,
): FactoryLayoutGroup | undefined {
  return (layout.groups ?? []).find((group) => group.id === groupId);
}

export function createFactoryLayoutGroupId(layout: FactoryLayout): string {
  const existingIds = new Set((layout.groups ?? []).map((group) => group.id));
  let index = (layout.groups ?? []).length + 1;
  while (existingIds.has(`group-${index}`)) {
    index += 1;
  }

  return `group-${index}`;
}

export function defaultFactoryLayoutGroupLabel(layout: FactoryLayout): string {
  const nextIndex = (layout.groups ?? []).length + 1;
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `Group ${nextIndex}`;
}

export function defaultFactoryLayoutGroupBounds(
  center: FactoryLayoutPoint,
): FactoryLayoutGroup["bounds"] {
  return {
    height: FACTORY_LAYOUT_GROUP_DEFAULT_SIZE.height,
    width: FACTORY_LAYOUT_GROUP_DEFAULT_SIZE.width,
    x: center.x - FACTORY_LAYOUT_GROUP_DEFAULT_SIZE.width / 2,
    y: center.y - FACTORY_LAYOUT_GROUP_DEFAULT_SIZE.height / 2,
  };
}

export function createFactoryLayoutGroup(input: {
  bounds: FactoryLayoutGroup["bounds"];
  color?: FactoryLayoutGroupColorToken;
  id: string;
  label?: string;
  layout: FactoryLayout;
}): FactoryLayoutGroup {
  const color = input.color ?? "primary";
  const group: FactoryLayoutGroup = {
    bounds: {
      height: input.bounds.height,
      width: input.bounds.width,
      x: input.bounds.x,
      y: input.bounds.y,
    },
    id: input.id,
    label: input.label ?? defaultFactoryLayoutGroupLabel(input.layout),
    nodeIds: [],
  };

  if (color !== undefined) {
    group.color = color;
  }

  return group;
}

export function addFactoryLayoutGroup(
  layout: FactoryLayout,
  group: FactoryLayoutGroup,
): FactoryLayout {
  return {
    ...layout,
    groups: [...(layout.groups ?? []), structuredClone(group)],
  };
}

export function updateFactoryLayoutGroup(
  layout: FactoryLayout,
  groupId: string,
  update: (group: FactoryLayoutGroup) => FactoryLayoutGroup,
): FactoryLayout {
  const groups = layout.groups ?? [];
  const groupIndex = groups.findIndex((group) => group.id === groupId);
  if (groupIndex < 0) {
    return layout;
  }

  const nextGroups = [...groups];
  nextGroups[groupIndex] = update(structuredClone(groups[groupIndex]));

  return {
    ...layout,
    groups: nextGroups,
  };
}

export function removeFactoryLayoutGroup(
  layout: FactoryLayout,
  groupId: string,
): FactoryLayout {
  const groups = (layout.groups ?? []).filter((group) => group.id !== groupId);
  return {
    ...layout,
    groups: groups.length > 0 ? groups : undefined,
  };
}

export function factoryLayoutGroupsEqual(
  left: FactoryLayoutGroup,
  right: FactoryLayoutGroup,
): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function factoryLayoutGroupCanvasNodeOptions(
  nodes: readonly FactoryGraphNode[],
): readonly FactoryLayoutGroupCanvasNodeOption[] {
  return [...nodes]
    .map((node) => ({
      id: node.id,
      label: node.label,
    }))
    .sort((left, right) => left.label.localeCompare(right.label));
}

export function factoryLayoutGroupContainsNode(
  group: FactoryLayoutGroup,
  nodeId: string,
): boolean {
  return (group.nodeIds ?? []).includes(nodeId);
}

export function removeNodeFromAllFactoryLayoutGroups(
  layout: FactoryLayout,
  nodeId: string,
  exceptGroupId?: string,
): FactoryLayout {
  let changed = false;
  const groups = (layout.groups ?? []).map((group) => {
    if (exceptGroupId !== undefined && group.id === exceptGroupId) {
      return group;
    }

    const nodeIds = group.nodeIds ?? [];
    if (!nodeIds.includes(nodeId)) {
      return group;
    }

    changed = true;
    return {
      ...group,
      nodeIds: nodeIds.filter((id) => id !== nodeId),
    };
  });

  if (!changed) {
    return layout;
  }

  return {
    ...layout,
    groups,
  };
}

export function addNodeToFactoryLayoutGroup(
  layout: FactoryLayout,
  groupId: string,
  nodeId: string,
): FactoryLayout {
  const layoutWithoutNode = removeNodeFromAllFactoryLayoutGroups(
    layout,
    nodeId,
  );

  return updateFactoryLayoutGroup(layoutWithoutNode, groupId, (group) => {
    const nodeIds = group.nodeIds ?? [];
    if (nodeIds.includes(nodeId)) {
      return group;
    }

    return {
      ...group,
      nodeIds: [...nodeIds, nodeId],
    };
  });
}

export function removeNodeFromFactoryLayoutGroup(
  layout: FactoryLayout,
  groupId: string,
  nodeId: string,
): FactoryLayout {
  return updateFactoryLayoutGroup(layout, groupId, (group) => {
    const nodeIds = group.nodeIds ?? [];
    if (!nodeIds.includes(nodeId)) {
      return group;
    }

    return {
      ...group,
      nodeIds: nodeIds.filter((id) => id !== nodeId),
    };
  });
}

export function clampFactoryLayoutGroupBounds(
  bounds: FactoryLayoutGroup["bounds"],
): FactoryLayoutGroup["bounds"] {
  return {
    height: Math.max(FACTORY_LAYOUT_GROUP_MIN_SIZE.height, bounds.height),
    width: Math.max(FACTORY_LAYOUT_GROUP_MIN_SIZE.width, bounds.width),
    x: bounds.x,
    y: bounds.y,
  };
}

export function moveFactoryLayoutGroupByDelta(
  layout: FactoryLayout,
  groupId: string,
  delta: FactoryLayoutPoint,
  resolvedNodePositions: ReadonlyMap<string, FactoryLayoutPoint> = new Map(),
): FactoryLayout {
  const group = factoryLayoutGroupById(layout, groupId);
  if (!group) {
    return layout;
  }

  const nextLayout = updateFactoryLayoutGroup(
    layout,
    groupId,
    (currentGroup) => ({
      ...currentGroup,
      bounds: {
        ...currentGroup.bounds,
        x: currentGroup.bounds.x + delta.x,
        y: currentGroup.bounds.y + delta.y,
      },
    }),
  );

  const memberNodeIds = group.nodeIds ?? [];
  if (memberNodeIds.length === 0) {
    return nextLayout;
  }

  return moveFactoryLayoutNodesByDelta(
    nextLayout,
    memberNodeIds,
    delta,
    resolvedNodePositions,
  );
}

export function resizeFactoryLayoutGroup(
  layout: FactoryLayout,
  groupId: string,
  bounds: FactoryLayoutGroup["bounds"],
): FactoryLayout {
  return updateFactoryLayoutGroup(layout, groupId, (group) => ({
    ...group,
    bounds: clampFactoryLayoutGroupBounds(bounds),
  }));
}
