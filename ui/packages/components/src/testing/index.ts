/** Stable category path for `@you-agent-factory/components/testing`. */
export const COMPONENTS_CATEGORY = "testing" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export {
  render,
  renderPackageComponent,
  screen,
  userEvent,
  waitFor,
  within,
} from "./render";
