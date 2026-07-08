/** Stable package identifier for `@you-agent-factory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

/** Dashboard anchor proving `@you-agent-factory/components` resolves through the workspace package path. */
export const dashboardComponentsPackageName: ComponentsPackageName =
  COMPONENTS_PACKAGE_NAME;
