/** Stable package identifier for `@you-agent-factory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

export {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  type ComponentCategoryExportPath,
} from "./category-paths";

export { DescriptionList } from "./data-display/description-list";
export type { DescriptionListProps } from "./data-display/description-list";

export { ActionRow } from "./layout/action-row";
export type { ActionRowProps } from "./layout/action-row";
export { SurfacePanel, surfacePanelVariants } from "./layout/surface-panel";
export type { SurfacePanelProps } from "./layout/surface-panel";

export { Code, Heading, Label, Text } from "./primitives/typography";
export type { CodeProps, HeadingProps, TextProps } from "./primitives/typography";
