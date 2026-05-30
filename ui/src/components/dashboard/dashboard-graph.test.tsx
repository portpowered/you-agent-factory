import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "bun:test";

import { DashboardGraphFrame } from "./dashboard-graph";

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
