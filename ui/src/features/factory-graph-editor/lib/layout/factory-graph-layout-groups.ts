import type { components } from "../../../../api/generated/openapi";
import type {
  FactoryLayout,
  FactoryLayoutPoint,
} from "./factory-graph-layout-operations";

export type FactoryLayoutGroup = NonNullable<
  components["schemas"]["Factory"]["layout"]
>["groups"] extends (infer TGroup)[] | undefined
  ? TGroup
  : never;

export const FACTORY_LAYOUT_GROUP_DEFAULT_SIZE = {
  height: 320,
  width: 480,
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
  return (
    color !== undefined && FACTORY_LAYOUT_GROUP_COLOR_TOKEN_SET.has(color)
  );
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
