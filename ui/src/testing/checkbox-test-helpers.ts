import { expect } from "vitest";

/** Assert the shared shadcn-style checkbox treatment on a native checkbox input. */
export function expectStyledCheckbox(checkbox: HTMLElement) {
  expect(checkbox.className).toContain("sr-only");
  const indicator = checkbox.nextElementSibling;
  expect(indicator?.className).toContain("peer-checked:bg-primary");
  expect(indicator?.querySelector("svg")).toBeTruthy();
}
