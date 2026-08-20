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

describe("Factory workstation authored semantics", () => {
  it("renders authored runtime and scheduling semantics as visible node metadata", () => {
    const repeater = renderWorkstationNode({
      workstationSemantics: repeaterAgentSemantics,
    });

    expect(
      repeater.container
        .querySelector("[data-workstation-title]")
        ?.getAttribute("title"),
    ).toBe("Agent");
    expect(repeater.queryByText("Repeater")).toBeNull();
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
});

describe("Factory workstation guard card semantics", () => {
  it("renders a single-line breaker card without limits or target details", () => {
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
    expect(loopBreaker.getByText("Breaker")).toBeTruthy();
    expect(loopBreaker.queryByText("Loop breaker")).toBeNull();
    expect(loopBreaker.queryByText("Target workstation")).toBeNull();
    expect(loopBreaker.queryByText("Visit limit")).toBeNull();
    expect(loopBreaker.queryByText("review")).toBeNull();
    expect(
      loopBreaker.container.querySelectorAll("[data-workstation-guard-row]"),
    ).toHaveLength(0);
    expect(guardCard?.className).toContain("truncate");
    expect(guardCard?.className).toContain("whitespace-nowrap");
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
    expect(guardCard?.getAttribute("data-workstation-guard-type")).toBe(
      "VISIT_COUNT",
    );
    expect(failedLoopBreaker.getByText("Breaker")).toBeTruthy();
    expect(failedLoopBreaker.container.querySelector("button")).toBeNull();
  });
});

describe("Factory workstation fallback and localization semantics", () => {
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
    expect(unknown.queryByText("Default scheduler")).toBeNull();
    expect(unknown.container.textContent?.toLowerCase()).not.toContain(
      "exhaustion",
    );
  });

  it("localizes concise workstation semantics in zh-CN", () => {
    const localized = renderWorkstationNode({
      locale: "zh-CN",
      workstationSemantics: repeaterAgentSemantics,
    });

    expect(
      localized.container
        .querySelector("[data-workstation-title]")
        ?.getAttribute("title"),
    ).toBe("代理");
    expect(localized.queryByText("重复器")).toBeNull();
    expect(
      localized.container.querySelector("[data-workstation-semantic-icon]"),
    ).toBeNull();

    const defaultScheduler = renderWorkstationNode({ locale: "zh-CN" });
    expect(defaultScheduler.queryByText("默认调度器")).toBeNull();
  });
});

describe("Factory workstation density", () => {
  it("keeps the default scheduler single-line beside a guarded card", () => {
    const unknownScheduling = renderWorkstationNode({
      expanded: true,
      workstationSemantics: {
        ...loopBreakerSemantics,
        schedulingBehavior: "UNKNOWN",
      },
    });

    const scheduler = unknownScheduling.container.querySelector(
      "[data-workstation-scheduling-label]",
    );
    expect(scheduler?.textContent).toContain("Default scheduler");
    expect(scheduler?.className).toContain("truncate");
    expect(scheduler?.className).toContain("whitespace-nowrap");
  });

  it("does not put a disabled summary button over read-only graph nodes", () => {
    const readOnlySummary = renderWorkstationNode({
      expanded: true,
      summaryOnly: true,
    });

    expect(readOnlySummary.container.querySelector("button")).toBeNull();
    expect(
      readOnlySummary.container.querySelector(
        "[data-workstation-scheduling-label]",
      )?.textContent,
    ).toContain("Default scheduler");
  });

  it("keeps the interaction overlay and workstation selection control usable", () => {
    const onSelectWorkstation = vi.fn();
    const workstationNode = renderWorkstationNode({
      interactionOverlay: {
        badges: [{ label: "Connection ready", tone: "info" }],
        connectionHint: "Choose a connection point",
      },
      onSelectWorkstation,
      workstationSemantics: loopBreakerSemantics,
    });

    expect(
      workstationNode.container.querySelector(
        "[data-graph-interaction-overlay]",
      ),
    ).toBeTruthy();
    expect(
      workstationNode.container.querySelector("[data-workstation-guard-card]"),
    ).toBeTruthy();
    const button = workstationNode.getByRole("button", {
      name: "Select Draft workstation",
    });
    expect(button.getAttribute("type")).toBe("button");
    button.focus();
    expect(document.activeElement).toBe(button);
    button.click();
    expect(onSelectWorkstation).toHaveBeenCalledWith("workstation:draft");
  });
});
