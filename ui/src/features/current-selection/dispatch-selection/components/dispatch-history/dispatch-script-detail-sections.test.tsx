import { render, screen } from "@testing-library/react";
import { WIDGET_FRAME_BODY_TEXT_CLASS } from "@you-agent-factory/components/recipes";
import { describe, expect, it } from "vitest";

import {
  ScriptArgsSection,
  ScriptOutputSection,
} from "./dispatch-script-detail-sections";

describe("dispatch script detail sections", () => {
  it("renders script args as current-selection code values", () => {
    render(<ScriptArgsSection args={["--work", "work-1"]} label="Args" />);

    expect(screen.getByText("Args").className).toContain("af-supporting-label");
    expect(screen.getByText("--work").tagName).toBe("CODE");
    expect(screen.getByText("work-1").className).toContain("af-body-code");
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
    expect(screen.getByText("hello").className).toContain("text-code-medium");

    rerender(
      <ScriptOutputSection
        emptyMessage="No output recorded."
        label="Stdout"
        value={undefined}
      />,
    );

    expect(screen.getByText("No output recorded.").className).toContain(
      WIDGET_FRAME_BODY_TEXT_CLASS,
    );
  });
});
