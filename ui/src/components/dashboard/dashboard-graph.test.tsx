import { render, screen } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import {
  DashboardGraphBackground,
  DashboardGraphControls,
  DashboardGraphFrame,
} from "./dashboard-graph";

describe("dashboard graph chrome", () => {
  it("styles React Flow canvas and controls with Material role CSS variables", () => {
    render(
      <ReactFlowProvider>
        <DashboardGraphBackground />
        <DashboardGraphControls fitViewOptions={{}} />
      </ReactFlowProvider>,
    );

    const backgroundPattern = document.querySelector(
      ".react-flow__background pattern",
    );
    expect(backgroundPattern).not.toBeNull();
    expect(
      document
        .querySelector<SVGElement>(".react-flow__background")
        ?.style.getPropertyValue("--xy-background-pattern-color-props"),
    ).toBe("var(--color-outline)");

    const controls = document.querySelector(".react-flow__controls");
    expect(controls).not.toBeNull();
    expect((controls as HTMLElement | null)?.style.backgroundColor).toBe(
      "var(--color-af-graph-controls-surface)",
    );
    expect((controls as HTMLElement | null)?.style.border).toBe(
      "1px solid var(--color-af-graph-controls-border)",
    );

    const zoomIn = screen.getByRole("button", { name: "Zoom In" });
    expect(zoomIn).not.toBeNull();
    expect(
      (controls as HTMLElement | null)?.style.getPropertyValue(
        "--xy-controls-button-background-color",
      ),
    ).toBe("var(--color-af-graph-controls-button-surface)");
    expect(
      (controls as HTMLElement | null)?.style.getPropertyValue(
        "--xy-controls-button-border-color",
      ),
    ).toBe("var(--color-af-graph-controls-border)");
    expect(
      (controls as HTMLElement | null)?.style.getPropertyValue(
        "--xy-controls-button-color",
      ),
    ).toBe("var(--color-af-graph-controls-text)");
  });
});

describe("DashboardGraphFrame", () => {
  it("provides concrete width constraints for React Flow parents", () => {
    render(
      <DashboardGraphFrame aria-label="Graph frame">
        <div />
      </DashboardGraphFrame>,
    );

    const frame = screen.getByRole("region", { name: "Graph frame" });
    expect(frame.className).toContain("w-full");
    expect(frame.className).toContain("min-w-0");
    expect(frame.className).toContain("shadow-none");
    expect(frame.className).not.toContain("h-full");
    expect(frame.className).not.toContain("shadow-af-card");
    expect(frame.className).not.toContain("shadow-af-panel");
  });
});
