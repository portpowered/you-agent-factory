import type { GraphNodeHandle } from "./graph-node-handle";
import type { GraphNodeState } from "./graph-node-state";

export const genericGraphHandles: GraphNodeHandle[] = [
  {
    id: "input-target",
    buttonAriaLabel: "Input connection",
    label: "Input",
    side: "left",
    tone: "input",
    type: "target",
  },
  {
    id: "output-source",
    buttonAriaLabel: "Output connection",
    label: "Output",
    side: "right",
    tone: "output",
    type: "source",
  },
];

const [genericGraphInput, genericGraphOutput] = genericGraphHandles;

if (!genericGraphInput || !genericGraphOutput) {
  throw new Error("genericGraphHandles fixture is incomplete");
}

export const genericGraphInputHandle: GraphNodeHandle[] = [genericGraphInput];

export const genericGraphOutputHandle: GraphNodeHandle[] = [genericGraphOutput];

export type GraphInteractiveFixtureNode = {
  fixedState?: GraphNodeState;
  handles: GraphNodeHandle[];
  id: string;
  label: string;
  nodeKind: string;
  position: { x: number; y: number };
  selectable?: boolean;
  stateLabel?: string;
};

export const desktopInteractiveGraphNodes: GraphInteractiveFixtureNode[] = [
  {
    handles: genericGraphOutputHandle,
    id: "ready-node",
    label: "Ready node",
    nodeKind: "ready",
    position: { x: 24, y: 32 },
    selectable: true,
  },
  {
    handles: genericGraphInputHandle,
    id: "target-node",
    label: "Target node",
    nodeKind: "target",
    position: { x: 360, y: 32 },
    selectable: true,
  },
  {
    fixedState: "disabled",
    handles: genericGraphHandles,
    id: "disabled-node",
    label: "Disabled node",
    nodeKind: "disabled",
    position: { x: 24, y: 168 },
    stateLabel: "Disabled node",
  },
  {
    fixedState: "loading",
    handles: genericGraphHandles,
    id: "loading-node",
    label: "Loading node",
    nodeKind: "loading",
    position: { x: 192, y: 168 },
    stateLabel: "Loading node",
  },
  {
    fixedState: "error",
    handles: genericGraphHandles,
    id: "error-node",
    label: "Error node",
    nodeKind: "error",
    position: { x: 360, y: 168 },
    stateLabel: "Connection failed",
  },
];

export const narrowInteractiveGraphNodes: GraphInteractiveFixtureNode[] = [
  {
    handles: genericGraphOutputHandle,
    id: "ready-node",
    label: "Ready node",
    nodeKind: "ready",
    position: { x: 16, y: 24 },
    selectable: true,
  },
  {
    handles: genericGraphInputHandle,
    id: "target-node",
    label: "Target node",
    nodeKind: "target",
    position: { x: 16, y: 132 },
    selectable: true,
  },
  {
    fixedState: "disabled",
    handles: genericGraphHandles,
    id: "disabled-node",
    label: "Disabled node",
    nodeKind: "disabled",
    position: { x: 16, y: 240 },
    stateLabel: "Disabled node",
  },
  {
    fixedState: "loading",
    handles: genericGraphHandles,
    id: "loading-node",
    label: "Loading node",
    nodeKind: "loading",
    position: { x: 16, y: 348 },
    stateLabel: "Loading node",
  },
  {
    fixedState: "error",
    handles: genericGraphHandles,
    id: "error-node",
    label: "Error node",
    nodeKind: "error",
    position: { x: 16, y: 456 },
    stateLabel: "Connection failed",
  },
];
