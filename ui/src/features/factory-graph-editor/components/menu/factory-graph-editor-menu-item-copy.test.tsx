import { render, screen } from "@testing-library/react";

import { FactoryGraphEditorMenuItemCopy } from "../menu/factory-graph-editor-menu-item-copy";

describe("FactoryGraphEditorMenuItemCopy", () => {
  it("uses dashboard typography for menu item label and description", () => {
    render(
      <FactoryGraphEditorMenuItemCopy
        description="Creates an execution workstation."
        label="Workstation"
      />,
    );

    const label = screen.getByText("Workstation");
    const description = screen.getByText("Creates an execution workstation.");

    expect(label.tagName).toBe("SPAN");
    expect(label.className).toContain("af-body-text");
    expect(label.className).toContain("font-semibold");
    expect(label.className).toContain("text-on-surface");
    expect(description.className).toContain("af-supporting-text");
  });

  it("renders label-only menu items without extra copy", () => {
    render(<FactoryGraphEditorMenuItemCopy label="Work type" />);

    expect(screen.getByText("Work type")).toBeTruthy();
    expect(screen.queryByText("Creates an execution workstation.")).toBeNull();
  });
});
