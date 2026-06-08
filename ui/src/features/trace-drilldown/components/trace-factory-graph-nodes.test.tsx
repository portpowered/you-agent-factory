import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TraceDispatchFlowNode } from "../lib/trace-dispatch-factory-graph-flow";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";
import { WORK_RELATION_NODE_TYPES } from "../../graphs/public";
import { TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES } from "./trace-dispatch-factory-graph-node";

const DispatchNode = TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES.workstation;
const RelationNode = WORK_RELATION_NODE_TYPES.workRelation;

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
      "af-current-activity-node-surface-neutral",
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
      "af-current-activity-node-surface-neutral",
    );
    expect(node.className).toContain("border-info-border");
    expect(node.className).not.toContain("shadow-af-card");
    expect(node.className).not.toContain("shadow-af-panel");
  });
});

describe("Trace relation factory graph node", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders only the work label while preserving neutral relation chrome", () => {
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

    expect(screen.getByText("Story A")).toBeTruthy();
    expect(screen.queryByText("Work type")).toBeNull();
    expect(screen.queryByText("Depends on")).toBeNull();
    const node = screen.getByText("Story A").closest("article");
    if (!node) {
      throw new Error("Expected relation node shell to render.");
    }
    expect(node.className).toContain("border-outline");
    expect(node.className).toContain("bg-surface");
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
    expect(node.className).toContain("border-af-danger-border");
    expect(node.className).toContain("shadow-none");
    expect(node.className).not.toContain("shadow-af-card");
    expect(node.className).not.toContain("shadow-af-panel");
  });
});
