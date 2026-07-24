import {
  type RenderOptions,
  type RenderResult,
  render,
} from "@testing-library/react";
import type { ReactElement } from "react";

export function renderPackageComponent(
  ui: ReactElement,
  options?: Omit<RenderOptions, "wrapper">,
): RenderResult {
  return render(ui, options);
}

export {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
export { default as userEvent } from "@testing-library/user-event";
