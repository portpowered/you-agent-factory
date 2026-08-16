import { render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import type { FactoryGraphWorkstationSemantics } from "@you-agent-factory/factory-graph";
import { describe, expect, it, vi } from "vitest";
import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import {
  WorkstationKind,
  WorkstationType,
} from "../../../../api/generated/openapi";
import {
  type CurrentActivityWorkstationNode,
  WorkstationNodeView,
} from "../../../graphs/components/workstation-node-view";

vi.mock("@xyflow/react", () => ({
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  Position: { Left: "left", Right: "right" },
}));

const workstation: DashboardWorkstationNode = {
  node_id: "workstation:draft",
  workstation_kind: WorkstationKind.STANDARD,
  workstation_name: "Draft",
};

const repeaterAgentSemantics: FactoryGraphWorkstationSemantics = {
  controlRole: "NONE",
  runtimeRole: "AGENT",
  runtimeType: WorkstationType.AGENT_RUN,
  schedulingBehavior: WorkstationKind.REPEATER,
};

const loopBreakerSemantics: FactoryGraphWorkstationSemantics = {
  controlRole: "LOOP_BREAKER",
  guardedControl: {
    guardType: "VISIT_COUNT",
    limit: { fixed: 3 },
    targetWorkstation: "review",
  },
  runtimeRole: "LOGICAL_MOVE",
  runtimeType: WorkstationType.LOGICAL_MOVE,
  schedulingBehavior: WorkstationKind.STANDARD,
};

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

describe("Factory workstation node semantics", () => {
  it("renders authored runtime and scheduling semantics as visible node metadata", () => {
    const repeater = renderWorkstationNode({
      workstationSemantics: repeaterAgentSemantics,
    });

    expect(repeater.getByText("Agent")).toBeTruthy();
    expect(repeater.getByText("Repeater")).toBeTruthy();
    expect(
      repeater.container.querySelector("[data-workstation-semantic-icon]"),
    ).toBeNull();
    expect(
      repeater.container.querySelector(
        '[data-workstation-scheduling-behavior="REPEATER"]',
      ),
    ).toBeTruthy();
    expect(
      repeater.container.querySelector(
        '[data-workstation-runtime-type="AGENT_RUN"]',
      ),
    ).toBeTruthy();
  });

  it("renders a guarded logical move card without making it selectable", () => {
    const loopBreaker = renderWorkstationNode({
      workstationSemantics: loopBreakerSemantics,
    });
    const guardCard = loopBreaker.container.querySelector(
      "[data-workstation-guard-card]",
    );

    expect(guardCard).toBeTruthy();
    expect(guardCard?.getAttribute("data-workstation-guard-type")).toBe(
      "VISIT_COUNT",
    );
    expect(loopBreaker.getByText("Loop breaker")).toBeTruthy();
    expect(loopBreaker.getByText("Logical move")).toBeTruthy();
    expect(loopBreaker.getByText("review")).toBeTruthy();
    expect(loopBreaker.getByText("3")).toBeTruthy();
    expect(loopBreaker.container.querySelector("button")).toBeNull();

    const shell = loopBreaker.container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );
    expect(shell?.getAttribute("data-graph-visual-status")).toBe("quiet");
    expect(shell?.getAttribute("data-graph-visual-nested-accent")).toBe(
      "neutral",
    );
    expect(shell?.className).toContain(
      "[&_[data-workstation-guard-card]]:!border-outline",
    );
    expect(shell?.className).toContain(
      "[&_[data-workstation-guard-card]]:!bg-surface",
    );
    expect(shell?.className).toContain(
      "[&_[data-workstation-guard-card]]:!text-on-surface",
    );
  });

  it("derives a danger loop-breaker card from its failed parent workstation", () => {
    const failedLoopBreaker = renderWorkstationNode({
      runtimeStatus: "FAILED",
      workstationSemantics: loopBreakerSemantics,
    });
    const shell = failedLoopBreaker.container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );
    const guardCard = failedLoopBreaker.container.querySelector(
      "[data-workstation-guard-card]",
    );

    expect(shell?.getAttribute("data-graph-visual-status")).toBe("danger");
    expect(shell?.getAttribute("data-graph-visual-nested-accent")).toBe(
      "danger",
    );
    expect(shell?.className).toContain(
      "[&_[data-workstation-guard-card]]:!border-af-danger-border",
    );
    expect(shell?.className).toContain(
      "[&_[data-workstation-guard-card]]:!bg-error-container",
    );
    expect(shell?.className).toContain(
      "[&_[data-workstation-guard-card]]:!text-on-error-container",
    );
    expect(guardCard?.getAttribute("aria-label")).toBe("Loop breaker");
    expect(guardCard?.getAttribute("data-workstation-guard-type")).toBe(
      "VISIT_COUNT",
    );
    expect(failedLoopBreaker.getByText("review")).toBeTruthy();
    expect(failedLoopBreaker.getByText("3")).toBeTruthy();
    expect(failedLoopBreaker.container.querySelector("button")).toBeNull();
  });

  it("keeps missing semantic metadata neutral instead of inferring exhaustion", () => {
    const unknown = renderWorkstationNode();

    expect(
      unknown.container.querySelector(
        '[data-workstation-runtime-type="UNKNOWN"]',
      ),
    ).toBeTruthy();
    expect(
      unknown.container.querySelector(
        '[data-workstation-scheduling-behavior="UNKNOWN"]',
      ),
    ).toBeTruthy();
    expect(
      unknown.container.querySelector("[data-workstation-guard-card]"),
    ).toBeNull();
    expect(unknown.getByText("Default scheduler")).toBeTruthy();
    expect(unknown.container.textContent?.toLowerCase()).not.toContain(
      "exhaustion",
    );
  });

  it("localizes concise workstation semantics in zh-CN", () => {
    const localized = renderWorkstationNode({
      locale: "zh-CN",
      workstationSemantics: repeaterAgentSemantics,
    });

    expect(localized.getByText("代理")).toBeTruthy();
    expect(localized.getByText("重复器")).toBeTruthy();
    expect(
      localized.container.querySelector("[data-workstation-semantic-icon]"),
    ).toBeNull();

    const defaultScheduler = renderWorkstationNode({ locale: "zh-CN" });
    expect(defaultScheduler.getByText("默认调度器")).toBeTruthy();
  });
});
