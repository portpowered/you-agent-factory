import { render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { describe, expect, it, vi } from "vitest";

import type {
  DashboardPlaceRef,
  DashboardWorkstationNode,
} from "../../../api/dashboard/types";
import {
  type CurrentActivityWorkstationNode,
  WorkstationNodeView,
} from "../../graphs/components/workstation-node-view";
import {
  type CurrentActivityWorkTypeNode,
  WorkTypeNodeView,
} from "./current-activity-work-type-node";
import {
  type CurrentActivityWorkerNode,
  WorkerNodeView,
} from "./current-activity-worker-node";

vi.mock("@xyflow/react", () => ({
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  Position: { Left: "left", Right: "right" },
}));

const DEFAULT_HOVER_BG_CLASS = "hover:bg-warning-container";
const WORKSTATION_HOVER_BG_CLASS = "hover:bg-primary-container";
const MUTED_HOVER_OPACITY_CLASS = "hover:opacity-100";

const workerPlace: DashboardPlaceRef = {
  kind: "worker",
  place_id: "worker:writer",
  state_value: "writer",
};

const workTypePlace: DashboardPlaceRef = {
  kind: "constraint",
  place_id: "work-type:story",
  state_value: "story",
};

const workstation: DashboardWorkstationNode = {
  node_id: "workstation:draft",
  workstation_name: "Draft",
};

function renderWorkerNode(
  data: Partial<NodeProps<CurrentActivityWorkerNode>["data"]> = {},
) {
  return render(
    <WorkerNodeView
      data={{
        activeFlow: false,
        handles: [],
        kind: "worker",
        muted: false,
        place: workerPlace,
        selectedWorker: false,
        ...data,
      }}
      dragging={false}
      id="worker:writer"
      selected={false}
      type="worker"
      zIndex={0}
    />,
  );
}

function renderWorkTypeNode(
  data: Partial<NodeProps<CurrentActivityWorkTypeNode>["data"]> = {},
) {
  return render(
    <WorkTypeNodeView
      data={{
        activeFlow: false,
        handles: [],
        kind: "work-type",
        muted: false,
        place: workTypePlace,
        selectedWorkType: false,
        ...data,
      }}
      dragging={false}
      id="work-type:story"
      selected={false}
      type="workType"
      zIndex={0}
    />,
  );
}

function renderWorkstationNode(
  data: Partial<NodeProps<CurrentActivityWorkstationNode>["data"]> = {},
) {
  return render(
    <WorkstationNodeView
      data={{
        active: false,
        activeFlow: false,
        executions: [],
        handles: [],
        muted: false,
        now: Date.parse("2026-06-09T00:00:00Z"),
        selectedWorkID: null,
        selectedWorkstation: false,
        workstation,
        ...data,
      }}
      dragging={false}
      id="workstation:draft"
      selected={false}
      type="workstation"
      zIndex={0}
    />,
  );
}

describe("CurrentActivity node hover surfaces", () => {
  it("applies hover backgrounds to neutral worker, workstation, and work type nodes", () => {
    const workerShell = renderWorkerNode().container.querySelector(
      "[data-current-activity-node-type='worker']",
    );
    const workTypeShell = renderWorkTypeNode().container.querySelector(
      "[data-current-activity-node-type='workType']",
    );
    const workstationShell = renderWorkstationNode().container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );

    expect(workerShell?.className).toContain(DEFAULT_HOVER_BG_CLASS);
    expect(workTypeShell?.className).toContain(DEFAULT_HOVER_BG_CLASS);
    expect(workstationShell?.className).toContain(
      "bg-surface-container-highest",
    );
    expect(workstationShell?.className).toContain(WORKSTATION_HOVER_BG_CLASS);
  });

  it("keeps hover backgrounds when active work marks nodes active or muted", () => {
    const activeWorkerShell = renderWorkerNode({
      activeFlow: true,
    }).container.querySelector("[data-current-activity-node-type='worker']");
    const mutedWorkerShell = renderWorkerNode({
      muted: true,
    }).container.querySelector("[data-current-activity-node-type='worker']");
    const activeWorkTypeShell = renderWorkTypeNode({
      activeFlow: true,
    }).container.querySelector("[data-current-activity-node-type='workType']");
    const mutedWorkTypeShell = renderWorkTypeNode({
      muted: true,
    }).container.querySelector("[data-current-activity-node-type='workType']");
    const activeFlowShell = renderWorkstationNode({
      active: true,
      activeFlow: true,
    }).container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );
    const selectedWorkShell = renderWorkstationNode({
      active: true,
      selectedWorkID: "work-story-1",
    }).container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );
    const mutedWorkstationShell = renderWorkstationNode({
      muted: true,
    }).container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );

    expect(activeWorkerShell?.className).toContain(DEFAULT_HOVER_BG_CLASS);
    expect(mutedWorkerShell?.className).toContain(DEFAULT_HOVER_BG_CLASS);
    expect(mutedWorkerShell?.className).toContain(MUTED_HOVER_OPACITY_CLASS);
    expect(activeWorkTypeShell?.className).toContain(DEFAULT_HOVER_BG_CLASS);
    expect(mutedWorkTypeShell?.className).toContain(DEFAULT_HOVER_BG_CLASS);
    expect(mutedWorkTypeShell?.className).toContain(MUTED_HOVER_OPACITY_CLASS);
    expect(activeFlowShell?.className).toContain(WORKSTATION_HOVER_BG_CLASS);
    expect(selectedWorkShell?.className).toContain(WORKSTATION_HOVER_BG_CLASS);
    expect(mutedWorkstationShell?.className).toContain(
      WORKSTATION_HOVER_BG_CLASS,
    );
    expect(mutedWorkstationShell?.className).toContain(
      MUTED_HOVER_OPACITY_CLASS,
    );
  });
});
