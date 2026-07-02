/** Stable package identifier for `youagentfactory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "youagentfactory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

export {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  type ComponentCategoryExportPath,
} from "./category-paths";
