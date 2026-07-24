// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import {
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
} from "./widget-frame-content";
import {
  WidgetErrorState,
  WidgetLoadingState,
  WidgetSuccessState,
} from "./widget-frame-states";

describe("Widget frame state panels", () => {
  it("renders host-provided loading copy with busy status semantics", () => {
    renderPackageComponent(
      <WidgetLoadingState>
        <WidgetEmptyStateTitle>Loading content</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided loading message.
        </WidgetEmptyStateText>
      </WidgetLoadingState>,
    );

    const status = screen.getByRole("status");

    expect(status.getAttribute("aria-busy")).toBe("true");
    expect(
      screen.getByRole("heading", { name: "Loading content" }),
    ).toBeTruthy();
    expect(screen.getByText("Host-provided loading message.")).toBeTruthy();
    expect(status.querySelectorAll('[aria-hidden="true"]')).toHaveLength(4);
  });

  it("renders host-provided error copy with alert semantics", () => {
    renderPackageComponent(
      <WidgetErrorState>
        <WidgetEmptyStateTitle>Request failed</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided error message.
        </WidgetEmptyStateText>
      </WidgetErrorState>,
    );

    const alert = screen.getByRole("alert");

    expect(alert.className).toContain("bg-error-container");
    expect(
      screen.getByRole("heading", { name: "Request failed" }),
    ).toBeTruthy();
    expect(screen.getByText("Host-provided error message.")).toBeTruthy();
  });

  it("renders host-provided success copy with status semantics", () => {
    renderPackageComponent(
      <WidgetSuccessState>
        <WidgetEmptyStateTitle>Action completed</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided success message.
        </WidgetEmptyStateText>
      </WidgetSuccessState>,
    );

    const status = screen.getByRole("status");

    expect(status.className).toContain("bg-success-container");
    expect(
      screen.getByRole("heading", { name: "Action completed" }),
    ).toBeTruthy();
    expect(screen.getByText("Host-provided success message.")).toBeTruthy();
  });

  it("allows hosts to opt out of default loading placeholders", () => {
    renderPackageComponent(
      <WidgetLoadingState showDefaultPlaceholder={false}>
        <WidgetEmptyStateText>
          Loading without placeholders.
        </WidgetEmptyStateText>
      </WidgetLoadingState>,
    );

    expect(
      screen.getByRole("status").querySelector('[aria-hidden="true"]'),
    ).toBeNull();
    expect(screen.getByText("Loading without placeholders.")).toBeTruthy();
  });
});
