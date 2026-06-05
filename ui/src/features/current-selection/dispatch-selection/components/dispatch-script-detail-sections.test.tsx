import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  ScriptArgsSection,
  ScriptOutputSection,
} from "./dispatch-script-detail-sections";

describe("dispatch script detail sections", () => {
  it("renders script args as current-selection code values", () => {
    render(<ScriptArgsSection args={["--work", "work-1"]} label="Args" />);

    expect(screen.getByText("Args").className).toContain(
      "af-dashboard-supporting-label",
    );
    expect(screen.getByText("--work").tagName).toBe("CODE");
    expect(screen.getByText("work-1").className).toContain(
      "af-dashboard-body-code",
    );
  });

  it("omits empty arg sections", () => {
    const { container } = render(
      <ScriptArgsSection args={[]} label="Resolved args" />,
    );

    expect(container.firstChild).toBeNull();
  });

  it("renders script output through code panels with empty fallback copy", () => {
    const { rerender } = render(
      <ScriptOutputSection
        emptyMessage="No output recorded."
        label="Stdout"
        value="hello"
      />,
    );

    expect(screen.getByText("hello").tagName).toBe("PRE");
    expect(screen.getByText("hello").className).toContain(
      "af-dashboard-body-code",
    );

    rerender(
      <ScriptOutputSection
        emptyMessage="No output recorded."
        label="Stdout"
        value={undefined}
      />,
    );

    expect(screen.getByText("No output recorded.").className).toContain(
      "af-dashboard-body-text",
    );
  });
});
