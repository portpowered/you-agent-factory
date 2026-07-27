import { describe, expect, it } from "bun:test";
import { render, screen, within } from "@testing-library/react";

import { WorkflowActivityBentoShell } from "./workflow-activity-bento-shell";

describe("WorkflowActivityBentoShell contract", () => {
  it("owns the full-height clipping contract around the graph viewport", () => {
    render(
      <WorkflowActivityBentoShell
        headerAction={<button type="button">Edit graph</button>}
        title="Factory graph"
      >
        <section aria-label="Work graph viewport" />
      </WorkflowActivityBentoShell>,
    );

    const card = screen.getByRole("article", { name: "Factory graph" });
    const body = card.querySelector<HTMLElement>(
      "[data-workflow-activity-graph-body]",
    );

    expect(card.className).toContain("h-full");
    expect(card.className).toContain("max-h-full");
    expect(card.className).toContain("min-h-0");
    expect(card.className).toContain("overflow-hidden");
    expect(card.style.height).toBe("100%");
    expect(card.style.maxHeight).toBe("100%");
    expect(card.style.overflow).toBe("hidden");
    expect(body?.className).toContain("h-full");
    expect(body?.className).toContain("max-h-full");
    expect(body?.className).toContain("min-h-0");
    expect(body?.className).toContain("overflow-hidden");
    expect(body?.style.height).toBe("100%");
    expect(body?.style.maxHeight).toBe("100%");
    expect(body?.style.overflow).toBe("hidden");
    expect(
      within(card).getByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
  });

  it("composes feature actions into the shared compact card header", () => {
    render(
      <WorkflowActivityBentoShell
        headerAction={<button type="button">Remove card</button>}
        title="Factory graph"
      >
        <div />
      </WorkflowActivityBentoShell>,
    );

    const card = screen.getByRole("article", { name: "Factory graph" });
    const header = card.querySelector("header");

    expect(header).toBeTruthy();
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(
      within(header as HTMLElement).getByRole("button", {
        name: "Remove card",
      }),
    ).toBeTruthy();
  });
});
