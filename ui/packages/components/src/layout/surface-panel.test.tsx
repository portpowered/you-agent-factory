// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { Heading, Text } from "../primitives/typography";
import { renderPackageComponent, screen } from "../testing/render";
import { ActionRow } from "./action-row";
import { SurfacePanel, surfacePanelVariants } from "./surface-panel";

describe("SurfacePanel", () => {
  it("renders default high-surface panel styling", () => {
    renderPackageComponent(<SurfacePanel>Panel content</SurfacePanel>);

    const panel = screen.getByText("Panel content");
    expect(panel.className).toContain("border-outline");
    expect(panel.className).toContain("bg-surface-container-high");
    expect(panel.className).toContain("p-3");
    expect(panel.className).toContain("rounded-xl");
  });

  it("supports low, compact, no-padding, accent, and asChild variants", () => {
    renderPackageComponent(
      <SurfacePanel
        asChild
        className="custom-row"
        padding="none"
        radius="lg"
        surface="low"
        tone="accent"
      >
        <li>Projected row</li>
      </SurfacePanel>,
    );

    const row = screen.getByText("Projected row");
    expect(row.tagName).toBe("LI");
    expect(row.className).toContain("bg-surface-container-low");
    expect(row.className).toContain("rounded-lg");
    expect(row.className).toContain("border-primary");
    expect(row.className).toContain("custom-row");
    expect(row.className).not.toContain("p-2");
    expect(row.className).not.toContain("p-3");
  });

  it("preserves heading, content, and footer structure through host composition", () => {
    renderPackageComponent(
      <SurfacePanel className="grid gap-3" padding="compact" radius="lg">
        <Heading level="section">Panel heading</Heading>
        <Text>Panel body content</Text>
        <ActionRow actions={<button type="button">Save</button>} />
      </SurfacePanel>,
    );

    const panel = screen.getByText("Panel body content").parentElement;
    expect(panel?.className).toContain("border-outline");
    expect(panel?.className).toContain("rounded-lg");
    expect(panel?.className).toContain("p-2");
    expect(
      screen.getByRole("heading", { name: "Panel heading" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

  it("exposes variant class generation for non-component consumers", () => {
    expect(
      surfacePanelVariants({
        radius: "3xl",
      }),
    ).toContain("rounded-3xl");
    expect(
      surfacePanelVariants({
        padding: "compact",
        radius: "full",
        surface: "low",
        tone: "selected",
      }),
    ).toContain("rounded-full");
    expect(
      surfacePanelVariants({
        tone: "selected",
      }),
    ).toContain("bg-primary-container");
  });
});
