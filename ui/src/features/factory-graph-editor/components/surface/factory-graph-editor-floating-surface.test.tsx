import { render, screen } from "@testing-library/react";

import { FactoryGraphEditorFloatingSurface } from "../surface/factory-graph-editor-floating-surface";

describe("FactoryGraphEditorFloatingSurface", () => {
  it("projects shared floating panel styling onto graph editor sections", () => {
    render(
      <FactoryGraphEditorFloatingSurface
        aria-label="Graph tools"
        className="px-3"
        placement="bottomToolbar"
      >
        Tools
      </FactoryGraphEditorFloatingSurface>,
    );

    const surface = screen.getByRole("region", { name: "Graph tools" });

    expect(surface.className).toContain("rounded-full");
    expect(surface.className).toContain("border-outline");
    expect(surface.className).toContain("bg-surface-container-high");
    expect(surface.className).toContain("shadow-af-panel");
    expect(surface.className).toContain("bottom-4");
    expect(surface.className).toContain("px-3");
  });

  it("supports top-right floating panel placement", () => {
    render(
      <FactoryGraphEditorFloatingSurface
        aria-label="Visibility presets"
        placement="topRight"
      >
        Visibility
      </FactoryGraphEditorFloatingSurface>,
    );

    const surface = screen.getByRole("region", {
      name: "Visibility presets",
    });

    expect(surface.className).toContain("right-7");
    expect(surface.className).toContain("top-7");
  });
});
