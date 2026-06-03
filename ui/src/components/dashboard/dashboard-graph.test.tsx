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

    expect(source).toContain('color={DASHBOARD_GRAPH_BACKGROUND_COLOR}');
    expect(source).toContain("var(--color-outline)");
    expect(source).toContain("var(--color-surface-container-high)");
    expect(source).toContain("var(--color-on-surface-variant)");
    expect(source).toContain("var(--color-surface)");
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
    expect(frame.className).not.toContain("h-full");
  });
});
