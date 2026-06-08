import { expect } from "storybook/test";

/** Assert the shared shadcn-style checkbox treatment in Storybook play functions. */
export function expectStyledCheckboxInStory(checkbox: HTMLElement) {
  expect(checkbox.className).toContain("sr-only");
  const indicator = checkbox.nextElementSibling;
  expect(indicator?.className).toContain("peer-checked:bg-primary");
  expect(indicator?.querySelector("svg")).toBeTruthy();
}
