import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { WORK_RELATION_NODE_TYPES } from "../../graphs/components/work-relation-node";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";

const RelationNode = WORK_RELATION_NODE_TYPES.workRelation;

function relationNodeProps(
  data: TraceRelationFlowNode["data"],
): NodeProps<TraceRelationFlowNode> {
  return {
    data,
    dragging: false,
    id: data.endpointKey,
    isConnectable: false,
    selected: false,
    type: "workRelation",
    zIndex: 0,
  };
}

describe("Trace relation factory graph node rendering", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders only the work label with workstation-aligned relation chrome", () => {
    render(
      <RelationNode
        {...relationNodeProps({
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
        })}
      />,
    );

    expect(screen.getByText("Story A")).toBeTruthy();
    expect(screen.queryByText("Work type")).toBeNull();
    expect(screen.queryByText("Depends on")).toBeNull();
    const node = screen.getByText("Story A").closest("article");
    if (!node) {
      throw new Error("Expected relation node shell to render.");
    }
    expect(node.className).toContain("border-outline-variant");
    expect(node.className).toContain("bg-surface-container-highest");
    expect(node.className).toContain("border-info-border");
    expect(node.className).toContain("shadow-none");
    expect(node.className).not.toContain("shadow-af-card");
    expect(node.className).not.toContain("shadow-af-panel");
  });

  it("does not render semantic icons or metadata badges for non-work nodes", () => {
    render(
      <RelationNode
        {...relationNodeProps({
          connectionAnchors: [],
          displayLabel: "Worker A",
          endpointKey: "worker-a",
          factoryNodeId: "worker:worker-a",
          kind: "worker",
          kindLabel: "Worker",
          locale: "en",
          relationStates: [],
          relationTypes: [],
          selectable: false,
        })}
      />,
    );
    expect(screen.getByText("Worker A")).toBeTruthy();
    expect(screen.queryByLabelText("Worker")).toBeNull();

    cleanup();

    render(
      <RelationNode
        {...relationNodeProps({
          connectionAnchors: [],
          displayLabel: "GPU",
          endpointKey: "resource-gpu",
          factoryNodeId: "resource:gpu",
          kind: "resource",
          kindLabel: "Resource",
          locale: "en",
          relationStates: [],
          relationTypes: [],
          selectable: false,
        })}
      />,
    );
    expect(screen.getByText("GPU")).toBeTruthy();
    expect(screen.queryByLabelText("Resource")).toBeNull();
  });
});

describe("Trace relation factory graph node selection", () => {
  afterEach(() => {
    cleanup();
  });

  it("invokes onSelectWorkID for selectable relation endpoints", async () => {
    const onSelectWorkID = vi.fn();
    const user = userEvent.setup();

    render(
      <RelationNode
        {...relationNodeProps({
          connectionAnchors: [],
          displayLabel: "Story B",
          endpointKey: "work-b",
          factoryNodeId: "work-state:story:done",
          isSelectedWork: false,
          kind: "work-state",
          kindLabel: "Work state",
          locale: "en",
          onSelectWorkID,
          relationStates: ["FAILED"],
          relationTypes: ["RETRY"],
          selectable: true,
          workID: "work-b",
        })}
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
    expect(screen.queryByText("Failed")).toBeNull();
    expect(screen.queryByText("Retry")).toBeNull();
    const node = screen.getByText("Story B").closest("article");
    if (!node) {
      throw new Error("Expected relation node shell to render.");
    }
    expect(node.className).toContain("border-info-border");
    expect(node.className).toContain("bg-info-container");
    expect(node.className).toContain("shadow-none");
    expect(node.className).not.toContain("shadow-af-card");
    expect(node.className).not.toContain("shadow-af-panel");
  });
});
