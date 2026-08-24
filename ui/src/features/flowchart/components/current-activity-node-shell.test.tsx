import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  ActivityGraphNodeShell,
  type ZAxisIncompleteHints,
} from "./current-activity-node-shell";

vi.mock("@xyflow/react", () => ({
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  NodeResizeControl: ({
    children,
    className,
    position,
  }: {
    children?: ReactNode;
    className?: string;
    position: string;
  }) => (
    <div className={className} data-testid={`resize-${position}`}>
      {children}
    </div>
  ),
  Position: { Left: "left", Right: "right" },
  ResizeControlVariant: { Handle: "handle" },
  useUpdateNodeInternals: () => () => undefined,
}));

const Z_AXIS_HINT: ZAxisIncompleteHints = {
  accessibleLabel:
    "Configure stop words on this workstation before connecting Continue or Reject routes.",
  title:
    "Configure stop words on this workstation before connecting Continue or Reject routes.",
};

describe("ActivityGraphNodeShell z-axis incomplete hints", () => {
  it("renders handle rails inside the node shell and pads content around them", () => {
    const { container } = render(
      <ActivityGraphNodeShell
        handles={[
          {
            id: "left-target",
            label: "Left",
            side: "left",
            type: "target",
          },
          {
            id: "right-source",
            label: "Right",
            side: "right",
            type: "source",
          },
        ]}
        nodeType="worker"
      >
        <p>Worker</p>
      </ActivityGraphNodeShell>,
    );

    expect(
      container.querySelector('[data-node-handle-rail="left"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-node-handle-rail="right"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-node-handle-badge="left-target"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-node-handle-badge="right-source"]'),
    ).toBeTruthy();

    const shell = container.querySelector("[data-graph-node-family]");
    expect(shell?.getAttribute("data-graph-node-family")).toBe("worker");
    expect(shell?.getAttribute("data-graph-node-shape")).toBe("worker");

    const content = screen.getByText("Worker").parentElement;
    expect(content?.className).toContain("pl-5");
    expect(content?.className).toContain("pr-5");
    expect(content?.getAttribute("data-graph-node-content-inset")).toBe(
      "compact",
    );
  });

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

describe("ActivityGraphNodeShell resize visibility", () => {
  const resizeControls = {
    allowedAxes: { height: false, width: true },
    bounds: {
      maximum: { height: 144, width: 360 },
      minimum: { height: 58, width: 156 },
    },
    nodeId: "worker:writer",
  } as const;

  it.each([
    ["worker", true],
    ["workstation", false],
  ] as const)(
    "only renders the %s resize control when the node is selected: %s",
    (nodeType, selected) => {
      const { container } = render(
        <ActivityGraphNodeShell
          handles={[]}
          nodeType={nodeType}
          resizeControls={resizeControls}
          visualState={{ selected }}
        >
          <p>Node</p>
        </ActivityGraphNodeShell>,
      );

      const resizeControl = container.querySelector("[data-testid^='resize-']");
      if (selected) {
        expect(resizeControl).toBeTruthy();
      } else {
        expect(resizeControl).toBeNull();
      }
    },
  );
});
