import type { FactoryGraphEdge } from "./factory-graph-draft-types";
import {
  buildEdgeRemovalDescription,
  describeEdgeLabel,
} from "./factory-graph-editor-edge-removal-copy";

const workerKey = { kind: "worker", name: "writer" } as const;
const resourceKey = { kind: "resource", name: "gpu" } as const;
const workstationKey = { kind: "workstation", name: "review" } as const;
const queuedStateKey = {
  kind: "work-state",
  stateName: "queued",
  workTypeName: "story",
} as const;
const doneStateKey = {
  kind: "work-state",
  stateName: "done",
  workTypeName: "story",
} as const;
const storyTypeKey = { kind: "work-type", name: "story" } as const;

describe("factory graph editor edge removal copy", () => {
  it.each([
    [
      edge("worker-assignment", workerKey, workstationKey),
      "writer assignment",
      "This will unassign writer from review. The workstation will need another worker before topology save can succeed.",
    ],
    [
      edge("worker-resource", resourceKey, workerKey),
      "gpu resource link",
      "This will remove gpu from writer's available resources in the pending draft.",
    ],
    [
      edge("workstation-resource", resourceKey, workstationKey),
      "gpu resource link",
      "This will remove gpu from review's required resources in the pending draft.",
    ],
    [
      edge("workstation-input", queuedStateKey, workstationKey),
      "story:queued input route",
      "This will stop routing story:queued into review.",
    ],
    [
      edge("workstation-output", workstationKey, doneStateKey),
      "review success route",
      "This will remove the success route from review to story:done.",
    ],
    [
      edge("workstation-on-continue", workstationKey, queuedStateKey),
      "review continue route",
      "This will remove the continue route from review to story:queued.",
    ],
    [
      edge("workstation-on-failure", workstationKey, queuedStateKey),
      "review failure route",
      "This will remove the failure route from review to story:queued.",
    ],
    [
      edge("workstation-on-rejection", workstationKey, queuedStateKey),
      "review rejection route",
      "This will remove the rejection route from review to story:queued.",
    ],
    [
      edge("work-type-state", storyTypeKey, queuedStateKey),
      "story state membership",
      "",
    ],
  ] as const)("describes %s edge removal copy", (graphEdge, expectedLabel, expectedDescription) => {
    expect(describeEdgeLabel(graphEdge)).toBe(expectedLabel);
    expect(buildEdgeRemovalDescription(graphEdge)).toBe(expectedDescription);
  });

  it("describes destructive edge removal copy in a non-default locale", () => {
    const graphEdge = edge("worker-assignment", workerKey, workstationKey);

    expect(describeEdgeLabel(graphEdge, "zh-CN")).toBe("writer 分配");
    expect(buildEdgeRemovalDescription(graphEdge, "zh-CN")).toBe(
      "这会将 writer 从 review 取消分配。该工作站需要另一个工作者后拓扑保存才能成功。",
    );
  });
});

function edge(
  kind: FactoryGraphEdge["kind"],
  source: FactoryGraphEdge["source"],
  target: FactoryGraphEdge["target"],
): FactoryGraphEdge {
  return {
    id: `${kind}:test`,
    kind,
    source,
    sourceId: "source",
    target,
    targetId: "target",
  };
}
