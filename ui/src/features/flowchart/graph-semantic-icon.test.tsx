import { render, screen } from "@testing-library/react";

import {
  GRAPH_SEMANTIC_ICON_KINDS,
  GraphSemanticIcon,
  graphSemanticIconLabel,
} from "./graph-semantic-icon";

describe("GraphSemanticIcon", () => {
  it("renders every known graph semantic with an accessible SVG label", () => {
    render(
      <div>
        {GRAPH_SEMANTIC_ICON_KINDS.map((kind) => (
          <GraphSemanticIcon kind={kind} key={kind} />
        ))}
      </div>,
    );

    for (const kind of GRAPH_SEMANTIC_ICON_KINDS) {
      const icon = screen.getByRole("img", { name: graphSemanticIconLabel(kind) });

      expect(icon.tagName.toLowerCase()).toBe("svg");
      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(kind);
      expect(icon.getAttribute("stroke")).toBe("currentColor");
      expect(icon.getAttribute("class")).toContain("text-af-ink/72");
      expect(icon.querySelectorAll("path, rect, circle, ellipse").length).toBeGreaterThan(0);
    }
  });

  it("renders a deterministic fallback icon for future semantics", () => {
    render(<GraphSemanticIcon kind="future-node-kind" />);

    const icon = screen.getByRole("img", { name: "Unknown graph semantic" });

    expect(icon.getAttribute("data-graph-semantic-icon")).toBe("unknown");
    expect(icon.getAttribute("viewBox")).toBe("0 0 24 24");
    expect(icon.querySelectorAll("path")).toHaveLength(3);
  });

  it("allows callers to provide a precise accessible fallback label", () => {
    render(<GraphSemanticIcon kind="future-node-kind" label="Replay gate" />);

    expect(screen.getByRole("img", { name: "Replay gate" })).toBeTruthy();
  });
});

