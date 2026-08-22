import { cleanup, render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import { WorkstationKind } from "../../../../api/generated/openapi";
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

const execution = {
  dispatch_id: "dispatch-1",
  started_at: "2026-06-09T00:00:00Z",
  work_items: [{ work_id: "work-1" }],
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
        now: Date.parse("2026-06-09T00:01:00Z"),
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

function nodeShell(container: HTMLElement): HTMLElement | null {
  return container.querySelector(
    "[data-current-activity-node-type='workstation']",
  );
}

/** `!bg-warning` is a substring of `!bg-warning-container`; compare tokens. */
function shellClassTokens(shell: HTMLElement | null): string[] {
  return (shell?.className ?? "").split(" ").filter(Boolean);
}

/**
 * Built at runtime rather than spelled out, so a class the node must never
 * carry does not become a rule the class scanner emits from this file.
 */
function invertedInkClassName(attribute: string): string {
  return `[&_[${attribute}]]:!text-on-warning`;
}

describe("Factory workstation node fill by active work", () => {
  afterEach(() => {
    cleanup();
  });

  it("keeps an idle workstation translucent", () => {
    const shell = nodeShell(renderWorkstationNode().container);

    expect(shell?.getAttribute("data-graph-visual-fill")).toBe("soft");
    expect(shellClassTokens(shell)).not.toContain("!bg-warning");
    expect(shell?.className).not.toContain(
      invertedInkClassName("data-factory-entity-title"),
    );
  });

  it("fills a workstation solidly while it runs work", () => {
    const shell = nodeShell(
      renderWorkstationNode({ active: true, executions: [execution] })
        .container,
    );

    expect(shell?.getAttribute("data-graph-visual-fill")).toBe("solid");
    expect(shellClassTokens(shell)).toContain("!bg-warning");
    expect(shellClassTokens(shell)).not.toContain("!bg-warning-container");
  });

  it("inverts workstation title ink while it runs work", () => {
    const shell = nodeShell(
      renderWorkstationNode({ active: true, executions: [execution] })
        .container,
    );

    expect(shell?.className).toContain(
      "[&_[data-factory-entity-title]]:!text-on-warning",
    );
  });

  it("leaves work-item chip ink alone because the chip keeps its own surface", () => {
    const { container } = renderWorkstationNode({
      active: true,
      executions: [execution],
    });
    const shell = nodeShell(container);

    expect(
      container.querySelector("[data-active-work-label]")?.closest("div, a")
        ?.className ?? "",
    ).toContain("bg-surface");
    expect(shell?.className).not.toContain(
      invertedInkClassName("data-active-work-label"),
    );
    expect(shell?.className).not.toContain(
      invertedInkClassName("data-active-work-duration"),
    );
  });
});
