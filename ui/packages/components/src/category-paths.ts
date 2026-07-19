/** Planned deep import paths for `@you-agent-factory/components` category entrypoints. */
export const COMPONENT_CATEGORY_EXPORT_PATHS = [
  "primitives",
  "forms",
  "layout",
  "feedback",
  "data-display",
  "navigation",
  "overlays",
  "charts",
  "factory-emulator",
  "graphs",
  "recipes",
  "icons",
  "utilities",
  "testing",
  "tokens",
] as const;

export type ComponentCategoryExportPath =
  (typeof COMPONENT_CATEGORY_EXPORT_PATHS)[number];
