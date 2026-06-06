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

describe("Trace dispatch factory graph node", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders factory-style workstation identity without dispatch metadata", () => {
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

    expect(screen.getByText("Workstation")).toBeTruthy();
    expect(screen.queryByText("Observed")).toBeNull();
    expect(screen.queryByText(/^In:/)).toBeNull();
    expect(screen.queryByText(/^Out:/)).toBeNull();
    const node = screen.getByText("dispatch-pending").closest("article");
    if (!node) {
      throw new Error("Expected dispatch node shell to render.");
    }
    expect(node.className).toContain("border-primary");
    expect(node.className).toContain("bg-primary-container");
  });

  it("keeps the factory workstation surface for failure outcomes", () => {
    render(
      <DispatchNode
        {...dispatchNodeProps({
          connectionAnchors: [],
          dispatchId: "dispatch-failed",
          displayLabel: "dispatch-failed",
          factoryNodeId: "workstation:dispatch-failed",
          inputSummary: "None",
          kind: "workstation",
          kindLabel: "Workstation",
          locale: "en",
          outcome: "FAILED",
          outputSummary: "None",
          workstationName: "dispatch-failed",
        })}
      />,
    );

    const node = screen.getByText("dispatch-failed").closest("article");
    if (!node) {
      throw new Error("Expected dispatch node shell to render.");
    }
    expect(node.className).toContain("border-primary");
    expect(node.className).toContain("bg-primary-container");
  });
});

describe("Trace relation factory graph node", () => {
  afterEach(() => {
    cleanup();
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
    expect(node.className).toContain("border-outline");
    expect(node.className).toContain("bg-surface");
  });

  it("renders resource and worker relation semantic icons", () => {
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
    expect(
      screen.getByLabelText("Worker").getAttribute("data-graph-semantic-icon"),
    ).toBe("active-work");

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
    expect(
      screen
        .getByLabelText("Resource")
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
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
    expect(screen.getByText("Failed").className).toContain(
      "bg-error-container",
    );
    const node = screen.getByText("Story B").closest("article");
    if (!node) {
      throw new Error("Expected relation node shell to render.");
    }
    expect(node.className).toContain("border-af-danger-border");
  });
});
