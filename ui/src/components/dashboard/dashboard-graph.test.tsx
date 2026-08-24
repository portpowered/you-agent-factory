import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { DashboardGraphFrame } from "./dashboard-graph";

const dashboardGraphSourcePath = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "dashboard-graph.tsx",
);

describe("dashboard graph chrome", () => {
  it("styles React Flow canvas and controls with Material role CSS variables", () => {
    const source = readFileSync(dashboardGraphSourcePath, "utf8");

    expect(source).toContain("color={DASHBOARD_GRAPH_BACKGROUND_COLOR}");
    expect(source).toContain(
      "var(--color-af-graph-controls-button-surface)",
    );
    expect(source).toContain(
      "var(--color-af-graph-controls-button-surface-hover)",
    );
    expect(source).toContain("var(--color-af-graph-controls-border)");
    expect(source).toContain("var(--color-af-graph-controls-text)");
    expect(source).toContain("var(--color-af-graph-controls-text-hover)");
    expect(source).toContain("var(--color-af-graph-controls-surface)");
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
