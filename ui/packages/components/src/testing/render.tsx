import {
  render,
  type RenderOptions,
  type RenderResult,
} from "@testing-library/react";
import type { ReactElement } from "react";

export function renderPackageComponent(
  ui: ReactElement,
  options?: Omit<RenderOptions, "wrapper">,
): RenderResult {
  return render(ui, options);
}

export { fireEvent, render, screen, within, waitFor } from "@testing-library/react";
export { default as userEvent } from "@testing-library/user-event";
