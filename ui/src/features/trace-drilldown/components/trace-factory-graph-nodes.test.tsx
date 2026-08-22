import { cleanup, render, screen } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { FACTORY_GRAPH_NODE_TYPES } from "@you-agent-factory/factory-graph";
import { afterEach, describe, expect, it } from "vitest";

import type { TraceDispatchFlowNode } from "../lib/trace-dispatch-factory-graph-flow";

const DispatchNode = FACTORY_GRAPH_NODE_TYPES.workstation;

function dispatchNodeProps(
  data: TraceDispatchFlowNode["data"],
): NodeProps<TraceDispatchFlowNode> {
  return {
    data,
    dragging: false,
    id: data.dispatchId,
    isConnectable: false,
    selected: false,
    type: "workstation",
    zIndex: 0,
  };
}

function dispatchNodeData(
  overrides: Partial<TraceDispatchFlowNode["data"]> = {},
): TraceDispatchFlowNode["data"] {
  return {
    active: false,
    activeFlow: false,
    dispatchId: "dispatch",
    displayLabel: "dispatch",
    executions: [],
    handles: [],
    inputSummary: "None",
    kind: "workstation",
    locale: "en",
    muted: false,
    now: 0,
    outputSummary: "None",
    selectedWorkID: null,
    selectedWorkstation: false,
    workstation: {
      node_id: "dispatch",
      transition_id: "dispatch",
      workstation_kind: "STANDARD",
      workstation_name: "dispatch",
    },
    ...overrides,
  } as never;
}

describe("Trace dispatch factory graph node", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders factory-style workstation identity without dispatch metadata", () => {
    render(
      <DispatchNode
        {...dispatchNodeProps(
          dispatchNodeData({
            dispatchId: "dispatch-pending",
            displayLabel: "dispatch-pending",
            executions: [],
            factoryNodeId: "workstation:dispatch-pending",
            handles: [],
            inputSummary: "None",
            kind: "workstation",
            locale: "en",
            muted: false,
            now: 0,
            outputSummary: "None",
            selectedWorkID: null,
            selectedWorkstation: false,
            workstation: {
              node_id: "dispatch-pending",
              transition_id: "dispatch-pending",
              workstation_kind: "STANDARD",
              workstation_name: "dispatch-pending",
            },
          }),
        )}
      />,
    );

    expect(screen.queryByText("Workstation")).toBeNull();
    const node = screen.getByText("dispatch-pending").closest("article");
    if (!node) {
      throw new Error("Expected dispatch node shell to render.");
    }
    expect(node.className).toContain(
      "border-outline-variant bg-surface-container-highest",
    );
    expect(node.className).toContain("border-info-border");
    expect(node.className).not.toContain("shadow-af-card");
    expect(node.className).not.toContain("shadow-af-panel");
  });

  it("keeps the factory workstation surface for failure outcomes", () => {
    render(
      <DispatchNode
        {...dispatchNodeProps(
          dispatchNodeData({
            dispatchId: "dispatch-failed",
            displayLabel: "dispatch-failed",
            executions: [],
            factoryNodeId: "workstation:dispatch-failed",
            handles: [],
            inputSummary: "None",
            kind: "workstation",
            locale: "en",
            muted: false,
            now: 0,
            outcome: "FAILED",
            outputSummary: "None",
            selectedWorkID: null,
            selectedWorkstation: false,
            workstation: {
              node_id: "dispatch-failed",
              transition_id: "dispatch-failed",
              workstation_kind: "STANDARD",
              workstation_name: "dispatch-failed",
            },
          }),
        )}
      />,
    );

    const node = screen.getByText("dispatch-failed").closest("article");
    if (!node) {
      throw new Error("Expected dispatch node shell to render.");
    }
    expect(node.className).toContain(
      "border-outline-variant bg-surface-container-highest",
    );
    expect(node.className).toContain("border-info-border");
    expect(node.className).not.toContain("shadow-af-card");
    expect(node.className).not.toContain("shadow-af-panel");
  });
});
