// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { Skeleton } from "./skeleton";

describe("Skeleton", () => {
  it("renders a non-interactive loading placeholder", () => {
    renderPackageComponent(<Skeleton className="h-4 w-24" data-testid="skeleton" />);

    const skeleton = screen.getByTestId("skeleton");
    expect(skeleton).toHaveAttribute("aria-hidden", "true");
    expect(skeleton.className).toContain("animate-pulse");
    expect(skeleton.className).toContain("bg-af-overlay");
  });
});
