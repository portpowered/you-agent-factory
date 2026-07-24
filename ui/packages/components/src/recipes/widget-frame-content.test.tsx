// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetSubtitle,
} from "./widget-frame-content";

describe("WidgetEmptyState", () => {
  it("renders compact empty states through the package contract", () => {
    renderPackageComponent(
      <WidgetEmptyState compact>
        <h3>No item selected</h3>
      </WidgetEmptyState>,
    );

    const emptyHeading = screen.getByRole("heading", {
      name: "No item selected",
    });
    expect(emptyHeading.parentElement?.className).toContain("min-h-0");
    expect(emptyHeading.parentElement?.className).toContain(
      "bg-surface-container-low",
    );
  });

  it("renders empty-state title and body copy through shared typography roles", () => {
    renderPackageComponent(
      <WidgetEmptyState>
        <WidgetEmptyStateTitle as="h2">No chart data</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided empty-state guidance.
        </WidgetEmptyStateText>
      </WidgetEmptyState>,
    );

    const title = screen.getByRole("heading", {
      level: 2,
      name: "No chart data",
    });
    const body = screen.getByText("Host-provided empty-state guidance.");

    expect(title.className).toContain("text-title-large");
    expect(body.className).toContain("text-body-medium");
    expect(body.className).toContain("m-0");
  });
});

describe("WidgetSubtitle", () => {
  it("supports subtitle text on non-paragraph semantic elements", () => {
    renderPackageComponent(
      <dl>
        <WidgetSubtitle as="dd">42 completed</WidgetSubtitle>
      </dl>,
    );

    const value = screen.getByText("42 completed");

    expect(value.tagName).toBe("DD");
    expect(value.className).toContain("text-display-small");
  });
});

describe("WidgetDetailCopy", () => {
  it("renders host-provided body copy with widget frame typography", () => {
    renderPackageComponent(
      <WidgetDetailCopy>Host-provided detail copy.</WidgetDetailCopy>,
    );

    const copy = screen.getByText("Host-provided detail copy.");
    expect(copy.className).toContain("text-body-medium");
    expect(copy.className).toContain("m-0");
  });
});
