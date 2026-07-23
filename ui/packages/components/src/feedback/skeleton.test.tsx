// @vitest-environment happy-dom

import { axe } from "jest-axe";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { Skeleton } from "./skeleton";

describe("Skeleton", () => {
  it("renders a non-interactive loading placeholder with package tokens", () => {
    renderPackageComponent(
      <Skeleton className="h-4 w-24" data-testid="skeleton" />,
    );

    const skeleton = screen.getByTestId("skeleton");
    expect(skeleton).toHaveAttribute("aria-hidden", "true");
    expect(skeleton.className).toContain("animate-pulse");
    expect(skeleton.className).toContain("rounded-xl");
    expect(skeleton.className).toContain("bg-af-overlay");
    expect(skeleton.className).toContain("h-4");
    expect(skeleton.className).toContain("w-24");
  });

  it("merges consumer sizing classes without overriding package loading affordance", () => {
    renderPackageComponent(
      <Skeleton className="h-28 w-full max-w-48" data-testid="skeleton" />,
    );

    const skeleton = screen.getByTestId("skeleton");
    expect(skeleton.className).toContain("h-28");
    expect(skeleton.className).toContain("w-full");
    expect(skeleton.className).toContain("max-w-48");
    expect(skeleton.className).toContain("bg-af-overlay");
  });

  it("does not expose misleading content or interactive semantics to assistive technology", () => {
    renderPackageComponent(<Skeleton data-testid="skeleton" />);

    const skeleton = screen.getByTestId("skeleton");
    expect(skeleton).toHaveAttribute("aria-hidden", "true");
    expect(skeleton).not.toHaveAttribute("role");
    expect(skeleton).not.toHaveAttribute("tabindex");
    expect(skeleton.textContent).toBe("");
  });

  it("remains accessible when composed inside a busy loading region", async () => {
    renderPackageComponent(
      <section aria-busy="true" aria-label="Loading chart data">
        <div aria-hidden="true" className="grid w-full gap-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-28 w-full" />
        </div>
      </section>,
    );

    expect(screen.getByLabelText("Loading chart data")).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(await axe(document.body)).toHaveNoViolations();
  });
});
