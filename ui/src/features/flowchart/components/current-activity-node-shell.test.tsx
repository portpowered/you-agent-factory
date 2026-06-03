import { render, screen } from "@testing-library/react";

import {
  ActivityGraphNodeShell,
  type ZAxisIncompleteHints,
} from "./current-activity-node-shell";

vi.mock("@xyflow/react", () => ({
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  Position: { Left: "left", Right: "right" },
}));

const Z_AXIS_HINT: ZAxisIncompleteHints = {
  accessibleLabel:
    "Configure stop words on this workstation before connecting Continue or Reject routes.",
  title:
    "Configure stop words on this workstation before connecting Continue or Reject routes.",
};

describe("ActivityGraphNodeShell z-axis incomplete hints", () => {
  it("renders Continue and Reject hint orbs when zAxisIncompleteHints is set on workstations", () => {
    const { container } = render(
      <ActivityGraphNodeShell
        handles={[]}
        nodeType="workstation"
        zAxisIncompleteHints={Z_AXIS_HINT}
      >
        <p>Workstation</p>
      </ActivityGraphNodeShell>,
    );

    const hints = container.querySelectorAll("[data-z-axis-incomplete-hint]");
    expect(hints).toHaveLength(2);
    expect(
      container.querySelector(
        '[data-z-axis-incomplete-hint="workstation-on-continue-source"]',
      ),
    ).toBeTruthy();
    expect(
      container.querySelector(
        '[data-z-axis-incomplete-hint="workstation-on-rejection-source"]',
      ),
    ).toBeTruthy();

    for (const hint of hints) {
      expect(hint.getAttribute("aria-label")).toBe(Z_AXIS_HINT.accessibleLabel);
      expect(hint.getAttribute("title")).toBe(Z_AXIS_HINT.title);
      expect(hint.className).toContain("pointer-events-none");
      const orb = hint.querySelector("span[aria-hidden='true']");
      expect(orb?.className).toContain("bg-error");
      expect(orb?.className).toContain("animate-pulse");
      expect(orb?.className).toContain("border-af-danger-border");
    }
  });

  it("does not render z-axis hint orbs when hints are unset", () => {
    const { container } = render(
      <ActivityGraphNodeShell handles={[]} nodeType="workstation">
        <p>Workstation</p>
      </ActivityGraphNodeShell>,
    );

    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("does not render z-axis hint orbs on non-workstation node types", () => {
    const { container } = render(
      <ActivityGraphNodeShell
        handles={[]}
        nodeType="worker"
        zAxisIncompleteHints={Z_AXIS_HINT}
      >
        <p>Worker</p>
      </ActivityGraphNodeShell>,
    );

    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("exposes localized hint text to assistive tech and tooltips", () => {
    render(
      <ActivityGraphNodeShell
        handles={[]}
        nodeType="workstation"
        zAxisIncompleteHints={Z_AXIS_HINT}
      >
        <p>Workstation</p>
      </ActivityGraphNodeShell>,
    );

    const labeledHints = screen.getAllByRole("img", {
      name: Z_AXIS_HINT.accessibleLabel,
    });
    expect(labeledHints).toHaveLength(2);
  });
});
