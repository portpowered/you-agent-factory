import { render, screen } from "@testing-library/react";

import { GraphSemanticIcon } from "./index";

describe("flowchart public barrel", () => {
  it("renders GraphSemanticIcon with accessible output and semantic kind surface", () => {
    render(<GraphSemanticIcon kind="queue" />);

    const icon = screen.getByRole("img", { name: "Queue state" });

    expect(icon.tagName.toLowerCase()).toBe("svg");
    expect(icon.getAttribute("data-graph-semantic-icon")).toBe("queue");
  });
});
