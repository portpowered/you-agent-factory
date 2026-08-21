import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { DocNodeView } from "./current-activity-doc-node";

describe("DocNodeView", () => {
  it("renders an accessible doc node label and selection handler", async () => {
    const onSelectDoc = vi.fn();
    const user = userEvent.setup();

    render(
      <DocNodeView
        data={{
          displayLabel: "guide.md",
          factoryGraphNodeId: "doc:factory/docs/guide.md",
          handles: [],
          kind: "doc",
          onSelectDoc,
          selectedDoc: false,
          targetPath: "factory/docs/guide.md",
        }}
        id="doc:factory/docs/guide.md"
        type="doc"
      />,
    );

    expect(screen.getByText("guide.md")).toBeTruthy();
    expect(screen.getByText("factory/docs/guide.md")).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "Select guide.md doc" }),
    );

    expect(onSelectDoc).toHaveBeenCalledWith("factory/docs/guide.md");
  });

  it("renders a read-only doc node when selection is unavailable", () => {
    render(
      <DocNodeView
        data={{
          displayLabel: "guide.md",
          factoryGraphNodeId: "doc:factory/docs/guide.md",
          handles: [],
          kind: "doc",
          selectedDoc: true,
          targetPath: "factory/docs/guide.md",
        }}
        id="doc:factory/docs/guide.md"
        type="doc"
      />,
    );

    expect(screen.getByText("guide.md")).toBeTruthy();
    expect(screen.getByText("factory/docs/guide.md")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("renders the document detail projection after resize", () => {
    const { container } = render(
      <DocNodeView
        data={{
          displayLabel: "runbook.md",
          expanded: true,
          fileType: "DOC",
          factoryGraphNodeId: "doc:factory/docs/runbook.md",
          handles: [],
          kind: "doc",
          selectedDoc: false,
          targetPath: "factory/docs/runbook.md",
        }}
        id="doc:factory/docs/runbook.md"
        type="doc"
      />,
    );

    expect(
      container.querySelector('[data-factory-graph-expanded-content="doc"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-factory-graph-expanded-field="file-type"]')
        ?.textContent,
    ).toBe("DOC");
  });
});
