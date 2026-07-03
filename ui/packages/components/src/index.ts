/** Stable package identifier for `@you-agent-factory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

export {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  type ComponentCategoryExportPath,
} from "./category-paths";

export {
  AlertPanel,
  AlertPanelStatusLabel,
  AlertPanelText,
  AlertPanelTitle,
  Skeleton,
} from "./feedback";
export type {
  AlertPanelProps,
  AlertPanelSemanticVariant,
  AlertPanelStatusLabelProps,
  AlertPanelTextProps,
  AlertPanelTitleProps,
  AlertPanelTone,
  AlertPanelVariant,
} from "./feedback";
export { CodePanel, codePanelVariants } from "./data-display";
export type { CodePanelProps } from "./data-display";
