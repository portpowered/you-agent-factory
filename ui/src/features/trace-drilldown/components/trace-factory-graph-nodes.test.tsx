import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TraceDispatchFlowNode } from "../lib/trace-dispatch-factory-graph-flow";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";
import { TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES } from "./trace-dispatch-factory-graph-node";
import { TRACE_RELATION_FACTORY_GRAPH_NODE_TYPES } from "./trace-relation-factory-graph-node";

const DispatchNode = TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES.factoryEntity;
const RelationNode = TRACE_RELATION_FACTORY_GRAPH_NODE_TYPES.factoryEntity;

function dispatchNodeProps(
  data: TraceDispatchFlowNode["data"],
): NodeProps<TraceDispatchFlowNode> {
  return {
    data,
    dragging: false,
    id: data.dispatchId,
    isConnectable: false,
    selected: false,
    type: "factoryEntity",
    zIndex: 0,
  };
}

function relationNodeProps(
  data: TraceRelationFlowNode["data"],
): NodeProps<TraceRelationFlowNode> {
  return {
    data,
    dragging: false,
    id: data.endpointKey,
    isConnectable: false,
    selected: false,
    type: "factoryEntity",
    zIndex: 0,
  };
}

describe("Trace factory graph node components", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders neutral dispatch chrome when outcome is missing", () => {
    render(
      <DispatchNode
        {...dispatchNodeProps({
          connectionAnchors: [],
          dispatchId: "dispatch-pending",
          displayLabel: "dispatch-pending",
          factoryNodeId: "workstation:dispatch-pending",
          inputSummary: "None",
          kind: "workstation",
          kindLabel: "Workstation",
          locale: "en",
          outputSummary: "None",
          workstationName: "dispatch-pending",
        })}
      />,
    );

    expect(screen.getByText("Observed")).toBeTruthy();
    const node = screen.getByText("dispatch-pending").closest("article");
    if (!node) {
      throw new Error("Expected dispatch node shell to render.");
    }
    expect(node.className).toContain("border-af-border");
    expect(node.className).toContain("bg-af-surface");
  });

  it("renders warning dispatch chrome for CONTINUE outcomes", () => {
    render(
      <DispatchNode
        {...dispatchNodeProps({
          connectionAnchors: [],
          dispatchId: "dispatch-continue",
          displayLabel: "dispatch-continue",
          factoryNodeId: "workstation:dispatch-continue",
          inputSummary: "None",
          kind: "workstation",
          kindLabel: "Workstation",
          locale: "en",
          outcome: "CONTINUE",
          outputSummary: "None",
          workstationName: "dispatch-continue",
        })}
      />,
    );

    const node = screen.getByText("dispatch-continue").closest("article");
    if (!node) {
      throw new Error("Expected dispatch node shell to render.");
    }
    expect(node.className).toContain("border-af-warning-border");
    expect(node.className).toContain("bg-af-warning-surface");
  });

  it("renders work-type relation chrome with default tone when relation states are empty", () => {
    render(
      <RelationNode
        {...relationNodeProps({
          connectionAnchors: [],
          displayLabel: "Story A",
          endpointKey: "work-a",
          factoryNodeId: "work-type:story-a:work-a",
          kind: "work-type",
          kindLabel: "Work type",
          locale: "en",
          relationStates: [],
          relationTypes: ["DEPENDS_ON"],
          selectable: false,
        })}
      />,
    );

    expect(screen.getByText("Work type")).toBeTruthy();
    expect(screen.getByText("Depends on")).toBeTruthy();
    const node = screen.getByText("Story A").closest("article");
    if (!node) {
      throw new Error("Expected relation node shell to render.");
    }
    expect(node.className).toContain("border-af-border");
    expect(node.className).toContain("bg-af-surface");
  });

  it("invokes onSelectWorkID for selectable relation endpoints", () => {
    const onSelectWorkID = vi.fn();

    render(
      <RelationNode
        {...relationNodeProps({
          connectionAnchors: [],
          displayLabel: "Story B",
          endpointKey: "work-b",
          factoryNodeId: "work-state:story:done",
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

    fireEvent.click(screen.getByRole("button", { name: "Story B" }));
    expect(onSelectWorkID).toHaveBeenCalledWith("work-b");
    expect(screen.getByText("Failed")).toBeTruthy();
    const node = screen.getByText("Story B").closest("article");
    if (!node) {
      throw new Error("Expected relation node shell to render.");
    }
    expect(node.className).toContain("border-af-danger-border");
  });
});
