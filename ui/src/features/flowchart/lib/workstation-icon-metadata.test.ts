// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: workstation-kind regression matrix stays grouped around one metadata fixture.
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import {
  CRON_WORKSTATION_KIND,
  POLLER_WORKSTATION_KIND,
  REPEATER_WORKSTATION_KIND,
  STANDARD_WORKSTATION_KIND,
  SUPPORTED_WORKSTATION_ICON_METADATA,
  UNKNOWN_WORKSTATION_KIND,
  workstationIconMetadata,
  workstationSemanticKind,
} from "./workstation-icon-metadata";

function dashboardWorkstationNode(
  overrides: Partial<DashboardWorkstationNode> = {},
): DashboardWorkstationNode {
  return {
    node_id: "node-1",
    transition_id: "transition-1",
    workstation_kind: STANDARD_WORKSTATION_KIND,
    workstation_name: "Plan",
    worker_type: "processor",
    ...overrides,
  };
}

describe("workstationIconMetadata", () => {
  it("publishes the approved dashboard workstation icon vocabulary for supported kinds", () => {
    expect(SUPPORTED_WORKSTATION_ICON_METADATA).toEqual([
      {
        className: "text-on-surface-subtle",
        iconKind: "workstation",
        label: "Standard workstation",
        semanticKind: STANDARD_WORKSTATION_KIND,
      },
      {
        className: "text-info",
        iconKind: "repeater",
        label: "Repeater workstation",
        semanticKind: REPEATER_WORKSTATION_KIND,
      },
      {
        className: "text-success",
        iconKind: "cron",
        label: "Cron workstation",
        semanticKind: CRON_WORKSTATION_KIND,
      },
      {
        className: "text-primary",
        iconKind: "poller",
        label: "Poller workstation",
        semanticKind: POLLER_WORKSTATION_KIND,
      },
    ]);
  });

  it("maps canonical workstation kinds to one shared semantic icon contract", () => {
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({
          workstation_kind: STANDARD_WORKSTATION_KIND,
        }),
      ),
    ).toEqual({
      className: "text-on-surface-subtle",
      iconKind: "workstation",
      label: "Standard workstation",
      semanticKind: STANDARD_WORKSTATION_KIND,
    });
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({
          workstation_kind: REPEATER_WORKSTATION_KIND,
        }),
      ),
    ).toEqual({
      className: "text-info",
      iconKind: "repeater",
      label: "Repeater workstation",
      semanticKind: REPEATER_WORKSTATION_KIND,
    });
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: CRON_WORKSTATION_KIND }),
      ),
    ).toEqual({
      className: "text-success",
      iconKind: "cron",
      label: "Cron workstation",
      semanticKind: CRON_WORKSTATION_KIND,
    });
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: POLLER_WORKSTATION_KIND }),
      ),
    ).toEqual({
      className: "text-primary",
      iconKind: "poller",
      label: "Poller workstation",
      semanticKind: POLLER_WORKSTATION_KIND,
    });
  });

  it("normalizes legacy lowercase workstation kinds to the API enum icon contract", () => {
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: "repeater" }),
      ),
    ).toEqual({
      className: "text-info",
      iconKind: "repeater",
      label: "Repeater workstation",
      semanticKind: REPEATER_WORKSTATION_KIND,
    });
  });

  it("keeps missing and future workstation metadata neutral", () => {
    const missingMetadata = dashboardWorkstationNode({
      workstation_kind: "",
      worker_type: "",
    });

    expect(workstationSemanticKind(missingMetadata)).toBe(
      UNKNOWN_WORKSTATION_KIND,
    );
    expect(workstationIconMetadata(missingMetadata)).toEqual({
      className: "text-on-surface-subtle",
      iconKind: "workstation",
      label: "Unknown workstation semantics",
      semanticKind: UNKNOWN_WORKSTATION_KIND,
    });
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: "future-kind" }),
      ),
    ).toEqual({
      className: "text-on-surface-subtle",
      iconKind: "workstation",
      label: "Unknown workstation semantics",
      semanticKind: UNKNOWN_WORKSTATION_KIND,
    });
  });

  it("localizes workstation semantic labels without changing icon contracts", () => {
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({
          workstation_kind: REPEATER_WORKSTATION_KIND,
        }),
        "zh-CN",
      ),
    ).toEqual({
      className: "text-info",
      iconKind: "repeater",
      label: "重复器工作站",
      semanticKind: REPEATER_WORKSTATION_KIND,
    });
  });
});
