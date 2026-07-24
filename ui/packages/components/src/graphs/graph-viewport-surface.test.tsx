// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { renderPackageComponent, screen } from "../testing/render";
import { GraphViewportSurface } from "./graph-viewport-surface";

describe("GraphViewportSurface sizing", () => {
  it("honors explicit height utilities from className without collapsing to the parent", () => {
    renderPackageComponent(
      <GraphViewportSurface
        aria-label="Sized graph viewport"
        className="h-[28rem] w-full"
      >
        <p>Graph canvas</p>
      </GraphViewportSurface>,
    );

    const viewport = screen.getByRole("region", {
      name: "Sized graph viewport",
    });
    expect(viewport).toHaveClass("h-[28rem]");
    expect(viewport).not.toHaveClass("h-full");
    expect(viewport).not.toHaveClass("max-h-full");
  });

  it("allows hosts to opt into fill-height layout with h-full", () => {
    renderPackageComponent(
      <div className="h-64">
        <GraphViewportSurface
          aria-label="Fill-height graph viewport"
          className="h-full w-full"
        >
          <p>Graph canvas</p>
        </GraphViewportSurface>
      </div>,
    );

    const viewport = screen.getByRole("region", {
      name: "Fill-height graph viewport",
    });
    expect(viewport).toHaveClass("h-full");
    expect(viewport).not.toHaveClass("max-h-full");
  });
});
