import { render, screen } from "@testing-library/react";

import {
  ActivityGraphNodeBadge,
  activityGraphNodeSurfaceClassName,
} from "./current-activity-node-chrome";

describe("ActivityGraphNodeBadge", () => {
  it("renders semantic badge tones through shared graph chrome", () => {
    render(
      <ActivityGraphNodeBadge tone="warning">Pending</ActivityGraphNodeBadge>,
    );

    expect(screen.getByText("Pending").className).toContain(
      "bg-warning-container",
    );
  });
});

describe("activityGraphNodeSurfaceClassName", () => {
  it.each([
    ["danger", "border-af-danger-border bg-error-container"],
    ["info", "border-info-border bg-info-container"],
    ["neutral", "border-outline bg-surface"],
    ["neutralHigh", "border-outline-variant bg-surface-container-high"],
    ["primary", "border-primary bg-primary-container"],
    ["success", "border-af-success-border bg-success-container"],
    ["warning", "border-af-warning-border bg-warning-container"],
  ] as const)("maps %s graph surfaces", (tone, expectedClassName) => {
    expect(activityGraphNodeSurfaceClassName(tone)).toBe(expectedClassName);
  });
});
