import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";

import { FactoryGraphEditorMenuHeader } from "../menu/factory-graph-editor-menu-header";

describe("FactoryGraphEditorMenuHeader", () => {
  it("uses dashboard typography for graph editor popover headings", () => {
    render(
      <FactoryGraphEditorMenuHeader
        description="Choose what kind of node to add."
        title="Add node"
      />,
    );

    const title = screen.getByText("Add node");
    const description = screen.getByText("Choose what kind of node to add.");

    expect(title.tagName).toBe("P");
    expect(title.className).toContain("af-body-text");
    expect(title.className).toContain("font-semibold");
    expect(title.className).toContain("text-on-surface");
    expect(description.className).toContain("af-supporting-text");
  });

  it("omits optional description copy", () => {
    render(<FactoryGraphEditorMenuHeader title="Delete selection" />);

    expect(screen.getByText("Delete selection")).toBeInTheDocument();
    expect(screen.queryByText("Choose what kind of node to add.")).toBeNull();
  });
});
