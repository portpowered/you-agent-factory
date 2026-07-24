import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activityGraphNodeSurfaceClassName } from "../../flowchart/components/current-activity-node-chrome";
import type { WorkRelationNode } from "./work-relation-node";
import { WORK_RELATION_NODE_TYPES } from "./work-relation-node";

const RelationNode = WORK_RELATION_NODE_TYPES.workRelation;

function relationNodeProps(
  data: WorkRelationNode["data"],
): NodeProps<WorkRelationNode> {
  return {
    data,
    dragging: false,
    id: data.endpointKey ?? "work-a",
    isConnectable: false,
    selected: false,
    type: "workRelation",
    zIndex: 0,
  };
}

function relationNodeData(
  overrides: Partial<WorkRelationNode["data"]> = {},
): WorkRelationNode["data"] {
  return {
    connectionAnchors: [],
    displayLabel: "Story A",
    endpointKey: "work-a",
    factoryNodeId: "work-type:story-a:work-a",
    isSelectedWork: false,
    kind: "work-type",
    kindLabel: "Work type",
    locale: "en",
    relationStates: [],
    relationTypes: ["DEPENDS_ON"],
    selectable: false,
    ...overrides,
  };
}

describe("WorkRelationNodeView readability", () => {
  afterEach(() => {
    cleanup();
  });

  it("uses workstation-aligned surfaces for related work-type nodes", () => {
    render(<RelationNode {...relationNodeProps(relationNodeData())} />);

    const node = screen.getByText("Story A").closest("article");
    if (!node) {
      throw new Error("Expected relation node shell to render.");
    }

    expect(node.className).toContain("border-outline-variant");
    expect(node.className).toContain("bg-surface-container-highest");
    expect(node.className).toContain("border-info-border");
  });

  it("emphasizes the selected work item more strongly than related nodes", () => {
    const { rerender } = render(
      <RelationNode
        {...relationNodeProps(
          relationNodeData({
            displayLabel: "Active Story",
            isSelectedWork: true,
          }),
        )}
      />,
    );

    const selectedNode = screen.getByText("Active Story").closest("article");
    if (!selectedNode) {
      throw new Error("Expected selected relation node shell to render.");
    }

    expect(selectedNode.className).toContain("border-primary");
    expect(selectedNode.className).toContain("bg-primary-container");
    expect(selectedNode.className).toContain("shadow-af-accent-selected");

    rerender(
      <RelationNode
        {...relationNodeProps(
          relationNodeData({
            displayLabel: "Dependency Story",
            selectable: true,
            workID: "work-dependency-story",
          }),
        )}
      />,
    );

    const relatedNode = screen.getByText("Dependency Story").closest("article");
    if (!relatedNode) {
      throw new Error("Expected related relation node shell to render.");
    }

    expect(relatedNode.className).toContain("bg-info-container");
    expect(relatedNode.className).toContain("border-info-border");
    expect(relatedNode.className).not.toContain("shadow-af-accent-selected");
    expect(relatedNode.className).not.toContain(
      "border-primary bg-primary-container shadow-af-accent-selected",
    );
  });

  it("wraps long labels with a smaller readable font instead of truncating", () => {
    const longLabel =
      "Dependency Story With A Very Long Operator Readable Name";

    render(
      <RelationNode
        {...relationNodeProps(
          relationNodeData({
            displayLabel: longLabel,
          }),
        )}
      />,
    );

    const label = screen.getByText(longLabel);
    expect(label.className).toContain("break-words");
    expect(label.className).toContain("[overflow-wrap:anywhere]");
    expect(label.className).toContain("text-[0.72rem]");
    expect(label.className).not.toContain("truncate");
    expect(label.className).not.toContain("whitespace-nowrap");
  });

  it("keeps keyboard activation and focus styling for selectable related work", async () => {
    const onSelectWorkID = vi.fn();
    const user = userEvent.setup();

    render(
      <RelationNode
        {...relationNodeProps(
          relationNodeData({
            displayLabel: "Story B",
            onSelectWorkID,
            selectable: true,
            workID: "work-b",
          }),
        )}
      />,
    );

    const button = screen.getByRole("button", { name: "Story B" });
    fireEvent.click(button);
    button.focus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");

    expect(onSelectWorkID).toHaveBeenNthCalledWith(1, "work-b");
    expect(onSelectWorkID).toHaveBeenNthCalledWith(2, "work-b");
    expect(onSelectWorkID).toHaveBeenNthCalledWith(3, "work-b");
    expect(button.className).toContain("focus-visible:ring-af-focus-ring");
  });
});

describe("WorkRelationNodeView semantic surfaces", () => {
  afterEach(() => {
    cleanup();
  });

  it.each([
    ["FAILED", "danger", "bg-error-container"],
    ["DONE", "success", "bg-success-container"],
    ["PENDING", "warning", "bg-warning-container"],
  ] as const)(
    "maps %s relation states to %s semantic surfaces",
    (relationState, _tone, expectedBackgroundClass) => {
      render(
        <RelationNode
          {...relationNodeProps(
            relationNodeData({
              displayLabel: `${relationState} Story`,
              kind: "worker",
              relationStates: [relationState],
            }),
          )}
        />,
      );

      const node = screen
        .getByText(`${relationState} Story`)
        .closest("article");
      if (!node) {
        throw new Error("Expected relation node shell to render.");
      }

      expect(node.className).toContain(expectedBackgroundClass);
      expect(node.className).toContain(
        activityGraphNodeSurfaceClassName(_tone),
      );
    },
  );
});
