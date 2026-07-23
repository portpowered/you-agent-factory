import { render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { describe, expect, it, vi } from "vitest";

import type {
  DashboardPlaceRef,
  DashboardWorkstationNode,
} from "../../../api/dashboard/types";
import { WorkstationKind } from "../../../api/generated/openapi";
import {
  type CurrentActivityWorkstationNode,
  WorkstationNodeView,
} from "../../graphs/components/workstation-node-view";
import { activityGraphNodeSurfaceClassName } from "./current-activity-node-chrome";
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
  workstation_kind: WorkstationKind.STANDARD,
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

  it("keeps active workstation state visible without a trailing header badge", () => {
    const activeWorkstation = renderWorkstationNode({
      active: true,
      activeFlow: true,
      executions: [
        {
          dispatch_id: "dispatch-1",
          started_at: "2026-06-09T00:00:00Z",
          work_items: [{ work_id: "work-1", work_type: "story" }],
        },
      ],
      onSelectWorkstation: vi.fn(),
    });

    const workstationShell = activeWorkstation.container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );
    const headerButton = activeWorkstation.getByRole("button", {
      name: "Select Draft workstation",
    });

    expect(workstationShell?.className).toContain("border-af-success-border");
    expect(workstationShell?.className).toContain("ring-af-success-border");
    expect(
      activeWorkstation.container.querySelector("[data-active='true']"),
    ).toBeTruthy();
    expect(
      headerButton.querySelector("[data-graph-semantic-icon='active-work']"),
    ).toBeNull();
    expect(
      headerButton.querySelector("[data-workstation-semantic-icon]"),
    ).toBeTruthy();
    expect(
      headerButton.querySelector("[data-workstation-title]")?.textContent,
    ).toBe("Draft");
  });
});

describe("Active workstation work item rows", () => {
  it("uses compact spacing on active workstation work item rows", () => {
    const activeWorkstation = renderWorkstationNode({
      active: true,
      executions: [
        {
          dispatch_id: "dispatch-1",
          started_at: "2026-06-09T00:00:00Z",
          work_items: [
            {
              display_name: "Story Alpha",
              work_id: "work-1",
              work_type: "story",
            },
          ],
        },
        {
          dispatch_id: "dispatch-2",
          started_at: "2026-06-09T00:00:00Z",
          work_items: [
            {
              display_name: "Story Beta",
              work_id: "work-2",
              work_type: "story",
            },
          ],
        },
        {
          dispatch_id: "dispatch-3",
          started_at: "2026-06-09T00:00:00Z",
          work_items: [
            {
              display_name: "Story Gamma",
              work_id: "work-3",
              work_type: "story",
            },
          ],
        },
      ],
      onSelectWorkID: vi.fn(),
      selectedWorkID: "work-2",
    });

    const workItemButtons = [
      activeWorkstation.getByRole("button", { name: /Story Alpha/ }),
      activeWorkstation.getByRole("button", { name: /Story Beta/ }),
      activeWorkstation.getByRole("button", { name: /Story Gamma/ }),
    ];

    workItemButtons.forEach((button) => {
      expect(button.className).toContain("gap-1");
      expect(button.className).toContain("px-1.5");
      expect(button.className).toContain("py-1");
      expect(button.className).toContain("grid-cols-[minmax(0,1fr)_auto]");
      expect(button.className).toContain("overflow-hidden");
      expect(button.className).toContain("min-w-0");
      expect(
        button.querySelector("[data-active-work-label]")?.className,
      ).toContain("truncate");
      expect(
        button.querySelector("[data-active-work-duration]")?.className,
      ).toContain("shrink-0");
    });

    const selectedWorkButton = activeWorkstation.getByRole("button", {
      name: /Story Beta/,
      pressed: true,
    });
    expect(selectedWorkButton.className).toContain("border-info-border");
    expect(selectedWorkButton.className).toContain("bg-info-container");
    expect(selectedWorkButton.className).toContain("shadow-af-info-chip");
  });

  it("shows three-character-or-shorter graph duration tokens with full duration in title", () => {
    const activeWorkstation = renderWorkstationNode({
      active: true,
      executions: [
        {
          dispatch_id: "dispatch-1",
          started_at: "2026-06-09T00:00:00Z",
          work_items: [
            {
              display_name: "Story Alpha",
              work_id: "work-1",
              work_type: "story",
            },
          ],
        },
      ],
      now: Date.parse("2026-06-09T00:13:05Z"),
      onSelectWorkID: vi.fn(),
    });

    const workItemButton = activeWorkstation.getByRole("button", {
      name: /Story Alpha/,
    });
    const durationLabel = workItemButton.querySelector(
      "[data-active-work-duration]",
    );

    expect(durationLabel?.textContent).toBe("13m");
    expect(durationLabel?.textContent?.length).toBeLessThanOrEqual(3);
    expect(workItemButton.getAttribute("title")).toBe("Story Alpha - 13m 5s");
  });
});

describe("Active workstation label truncation", () => {
  it("keeps short workstation titles at the largest readable size", () => {
    const shortTitle = "Short Active Story";
    const activeWorkstation = renderWorkstationNode({
      active: true,
      workstation: {
        ...workstation,
        workstation_name: shortTitle,
      },
    });
    const titleLabel = activeWorkstation
      .getByText(shortTitle)
      .closest("[data-workstation-title]");

    expect(titleLabel?.className).toContain("text-[1rem]");
    expect(titleLabel?.className).toContain("truncate");
    expect(titleLabel?.className).toContain("whitespace-nowrap");
  });

  it("keeps medium workstation titles at the intermediate readable size", () => {
    const mediumTitle = "Review Requests With Medium Title";
    const activeWorkstation = renderWorkstationNode({
      active: true,
      workstation: {
        ...workstation,
        workstation_name: mediumTitle,
      },
    });
    const titleLabel = activeWorkstation
      .getByText(mediumTitle)
      .closest("[data-workstation-title]");

    expect(titleLabel?.className).toContain("text-[0.88rem]");
    expect(titleLabel?.className).not.toContain("text-[0.78rem]");
    expect(titleLabel?.className).toContain("truncate");
  });

  it("keeps medium active work labels at the default row size", () => {
    const mediumLabel = "Active Story With A Medium Sized Label";
    const activeWorkstation = renderWorkstationNode({
      active: true,
      executions: [
        {
          dispatch_id: "dispatch-1",
          started_at: "2026-06-09T00:00:00Z",
          work_items: [
            {
              display_name: mediumLabel,
              work_id: "work-1",
              work_type: "story",
            },
          ],
        },
      ],
      onSelectWorkID: vi.fn(),
    });
    const workItemButton = activeWorkstation.getByRole("button", {
      name: new RegExp(mediumLabel),
    });
    const workLabel = workItemButton.querySelector("[data-active-work-label]");
    const durationLabel = workItemButton.querySelector(
      "[data-active-work-duration]",
    );

    expect(workLabel?.className).toContain("text-[0.74rem]");
    expect(workLabel?.className).not.toContain("text-[0.68rem]");
    expect(workLabel?.className).toContain("truncate");
    expect(workLabel?.className).toContain("basis-0");
    expect(durationLabel?.className).toContain("shrink-0");
    expect(workItemButton.className).toContain("overflow-hidden");
  });
});

describe("Activity graph node surface tones", () => {
  it.each([
    ["worker", () => renderWorkerNode(), "bg-info-container"],
    ["workType", () => renderWorkTypeNode(), "bg-info-container"],
    [
      "workstation",
      () => renderWorkstationNode(),
      "bg-surface-container-highest",
    ],
  ] as const)(
    "applies shared surface mapping on %s nodes",
    (_nodeType, renderNode, expectedBackgroundClass) => {
      const shell = renderNode().container.querySelector("article");

      expect(shell?.className).toContain(expectedBackgroundClass);
      expect(shell?.className).toContain(
        activityGraphNodeSurfaceClassName(
          _nodeType === "workstation" ? "workstation" : "info",
        ),
      );
    },
  );

  it("keeps workstation active and selected overlays visible on mapped surfaces", () => {
    const activeWorkstationShell = renderWorkstationNode({
      active: true,
      activeFlow: true,
    }).container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );
    const selectedWorkstationShell = renderWorkstationNode({
      active: true,
      onSelectWorkstation: vi.fn(),
      selectedWorkstation: true,
    }).container.querySelector(
      "[data-current-activity-node-type='workstation']",
    );

    expect(activeWorkstationShell?.className).toContain(
      "bg-surface-container-highest",
    );
    expect(activeWorkstationShell?.className).toContain(
      "border-af-success-border",
    );
    expect(activeWorkstationShell?.className).toContain(
      "ring-af-success-border",
    );
    expect(selectedWorkstationShell?.className).toContain(
      "bg-surface-container-highest",
    );
    expect(selectedWorkstationShell?.className).toContain("border-primary");
    expect(selectedWorkstationShell?.className).toContain(
      "shadow-af-accent-selected",
    );
  });
});
